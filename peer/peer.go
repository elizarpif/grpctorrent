package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/elizarpif/logger"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"
)

type Peer struct {
	id        uuid.UUID
	hashFiles map[string]*file
	haveFiles map[string]*file
	tracker   api.TrackerClient
}

func NewPeer(ctx context.Context, trackerAddr, peerServerAddr string) (*Peer, error) {
	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithPerRPCCredentials(newAuth(peerServerAddr))}
	trackerClient, err := grpc.DialContext(ctx, trackerAddr, opts...)
	if err != nil {
		return nil, err
	}

	return &Peer{
		id:        uuid.New(),
		hashFiles: make(map[string]*file),
		haveFiles: make(map[string]*file),
		tracker:   api.NewTrackerClient(trackerClient),
	}, nil
}

func (p *Peer) UploadFile(ctx context.Context, f *api.File) (*empty.Empty, error) {
	logger.GetLogger(ctx).WithField("filename", f.Name).Debug("upload file")
	file, err := newFile(f.Name)
	if err != nil {
		return nil, err
	}

	hash := file.hash

	p.hashFiles[hash] = file
	p.haveFiles[file.name] = file

	_, err = p.tracker.Upload(ctx, &api.UploadFileRequest{
		ClientId:    p.id.String(),
		Name:        file.name,
		PieceLength: file.piecesLen,
		Pieces:      uint64(len(file.piecesMap)),
		Length:      file.length,
		Hash:        hash,
	})
	if err != nil {
		return nil, status.Error(codes.Canceled, "can't upload file to tracker")
	}

	return &empty.Empty{}, nil
}

func (p *Peer) GetFileInfo(ctx context.Context, f *api.File) (*api.FileInfo, error) {
	is, ok := p.haveFiles[f.Name]
	if ok {
		return &api.FileInfo{
			Name:        f.Name,
			PieceLength: is.piecesLen,
			Pieces:      uint64(len(is.piecesMap)),
			Length:      is.length,
			Hash:        is.hash,
		}, nil
	}

	logger.GetLogger(ctx).Error("cannot find file")
	return nil, status.Error(codes.NotFound, "cannot find file")
}

type downloadFields struct {
	isPieceDownload          map[uint64]bool
	position                 uint64
	anotherPeerAddr, hashStr string
	piecesMap                map[uint]*api.Piece
}

func (p *Peer) downloadPiece(ctx context.Context, df *downloadFields) error {
	position := df.position

	// если такой кусок еще не загружен
	if !df.isPieceDownload[df.position] {
		opts := []grpc.DialOption{grpc.WithInsecure()}
		conn, err := grpc.DialContext(ctx, df.anotherPeerAddr, opts...)
		if err != nil {
			return err
		}
		defer conn.Close()

		anotherPeer := api.NewPeerClient(conn)
		piece, err := anotherPeer.GetPiece(ctx, &api.GetPieceRequest{
			SerialNumber: df.position,
			Hash:         df.hashStr,
		})
		if err != nil {
			logger.GetLogger(ctx).WithError(err).WithField("remote_peer", df.anotherPeerAddr).Error("cannot get piece")
			return nil
		}

		df.piecesMap[uint(position)] = piece
		df.isPieceDownload[position] = true

		logger.GetLogger(ctx).
			WithField("peer_addr", df.anotherPeerAddr).
			WithField("position", position).
			Debug("download")

		_, err = p.tracker.PostPieceInfo(ctx, &api.PieceInfo{
			HashFile: df.hashStr,
			Serial:   df.position,
		})

		if err != nil {
			logger.GetLogger(ctx).WithError(err).Error("cannot post piece info")
			return err
		}
	}

	return nil
}

type fields struct {
	mutex           *sync.RWMutex
	addr            string
	positions       []uint64
	isPieceDownload map[uint64]bool
	file            *file
}

func (p *Peer) downloadAll(ctx context.Context, group *errgroup.Group, f *fields) {
	group.Go(func() error {
		f.mutex.Lock()

		anotherPeerAddr := f.addr
		hashStr := f.file.hash

		logger.GetLogger(ctx).WithField("addr", anotherPeerAddr).Debug("started cycle goroutine")

		f.mutex.Unlock()

		for _, position := range f.positions {
			f.mutex.Lock()

			err := p.downloadPiece(ctx, &downloadFields{
				isPieceDownload: f.isPieceDownload,
				position:        position,
				anotherPeerAddr: anotherPeerAddr,
				hashStr:         hashStr,
				piecesMap:       f.file.piecesMap,
			})

			if err != nil {
				f.mutex.Unlock()
				return err
			}

			if _, ok := p.hashFiles[hashStr]; !ok {
				p.hashFiles[hashStr] = f.file
			}
			if _, ok := p.haveFiles[f.file.name]; !ok {
				p.haveFiles[f.file.name] = f.file
			}

			f.mutex.Unlock()

			time.Sleep(time.Second)
		}

		return nil
	})
}

// TODO use goroutines
// TODO add logger in middleware grpc
func (p *Peer) Download(ctx context.Context, f *api.DownloadFileRequest) (*api.DownloadFileResponse, error) {
	hashStr := f.Hash

	info, err := p.tracker.GetFileInfo(ctx, &api.DownloadFileRequest{Hash: hashStr})
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// сходить на сервер и получить список пиров для файла
	list, err := p.tracker.GetPeers(ctx, &api.GetPeersRequest{
		HashFile: hashStr,
		PeerId:   p.id.String(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	file := &file{
		name:      info.Name,
		hash:      hashStr,
		allPieces: false,
		length:    info.Length,
		piecesLen: info.PieceLength,
		piecesMap: make(map[uint]*api.Piece), // карта кусков
	}

	// обратиться к клиенту и скачать по кусочкам

	isPieceDownload := make(map[uint64]bool)       // карта каждого куска
	peerAddrPositions := make(map[string][]uint64) // карта адреса пира к количеству достпуных кусок

	for _, p := range list.Peers {
		peerAddrPositions[p.Address] = p.SerialPieces
	}

	mutex := &sync.RWMutex{}
	group := &errgroup.Group{}

	logger.GetLogger(ctx).WithField("peer addr", peerAddrPositions).Debug("addresses")

	// пройтись по списку доступных пиров и скачать у них доступные файлы
	for anotherPeerAddr, positions := range peerAddrPositions {
		p.downloadAll(ctx, group, &fields{
			mutex:           mutex,
			addr:            anotherPeerAddr,
			positions:       positions,
			isPieceDownload: isPieceDownload,
			file:            file,
		})
	}

	err = group.Wait()
	if err != nil {
		logger.GetLogger(ctx).WithError(err).Error("error get pieces")
		return nil, status.Error(codes.Internal, "error in get pieces")
	}

	isAllFileDownload := true
	for _, piece := range file.piecesMap {
		if !isPieceDownload[piece.SerialNumber] {
			logger.GetLogger(ctx).WithField("piece_position", piece.SerialNumber).Error("file not downloaded")
			isAllFileDownload = false
		}
	}

	file.allPieces = isAllFileDownload

	err = file.MergePieces(ctx)
	if err != nil {
		logger.GetLogger(ctx).Error("cannot merge file")
		return nil, status.Error(codes.Internal, err.Error())
	}

	logger.GetLogger(ctx).WithField("filepath", file.name).Info("downloaded")

	return &api.DownloadFileResponse{
		FilePath: getDownloadFilename(file.name),
	}, nil
}

// пришел запрос "дай кусок"
func (p *Peer) GetPiece(ctx context.Context, request *api.GetPieceRequest) (*api.Piece, error) {
	log := logger.GetLogger(ctx)

	file, exists := p.hashFiles[request.Hash]
	if !exists {
		log.Error("file doesn't exists")
		return nil, errors.New("file doesn't exists")
	}

	piece, exists := file.piecesMap[uint(request.SerialNumber)]
	if !exists {
		log.WithField("serial", request.SerialNumber).WithField("map", file.piecesMap).Error("piece doesn't exists")
		return nil, errors.New("piece doesn't exists")
	}

	return piece, nil
}
