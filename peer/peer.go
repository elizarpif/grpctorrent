package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	getLogger(ctx).WithField("filename", f.Name).Debug("upload file")
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

	getLogger(ctx).Error("cannot find file")
	return nil, status.Error(codes.NotFound, "cannot find file")
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

	// обратиться к клиенту и скачать по кусочкам

	isPieceDownload := make(map[uint64]bool)       // карта каждого куска  
	peerAddrPositions := make(map[string][]uint64) // карта адреса пира к количеству достпуных кусок

	piecesMap := make(map[uint]*api.Piece) // карта кусков

	for _, p := range list.Peers {
		peerAddrPositions[p.Address] = p.SerialPieces
	}

	mutex := sync.RWMutex{}
	group := errgroup.Group{}

	// пройтись по списку доступных пиров и скачать у них доступные файлы
	for anotherPeerAddr, positions := range peerAddrPositions {
		group.Go(func() error {
			for _, position := range positions {
				// если такой кусок еще не загружен
				mutex.RLock()
				isDownload  := isPieceDownload[position]
				mutex.RUnlock()

				if !isDownload {

					opts := []grpc.DialOption{grpc.WithInsecure()}
					conn, err := grpc.DialContext(ctx, anotherPeerAddr, opts...)
					if err != nil {
						return err
					}
					defer conn.Close()

					anotherPeer := api.NewPeerClient(conn)
					piece, err := anotherPeer.GetPiece(ctx, &api.GetPieceRequest{
						SerialNumber: position,
						Hash:         hashStr,
					})
					if err == nil {
						mutex.Lock()

						piecesMap[uint(position)] = piece
						isPieceDownload[position] = true

						getLogger(ctx).
							WithField("peer_addr", anotherPeerAddr).
							WithField("position", position).
							Debug("download")

						_, err := p.tracker.PostPieceInfo(ctx, &api.PieceInfo{
							HashFile: hashStr,
							Serial:   position,
						})

						mutex.Unlock()

						if err != nil {
							getLogger(ctx).WithError(err).Error("cannot post piece info")
							return err
						}

						time.Sleep(time.Second)
					} else {
						mutex.Lock()
						getLogger(ctx).WithError(err).WithField("remote_peer", anotherPeerAddr).Error("cannot get piece")
						mutex.Unlock()
					}
				}

				mutex.RLock()
				lenDownloaded := len(isPieceDownload)
				if uint64(lenDownloaded) == info.Pieces {
					return nil
				}
				mutex.RUnlock()
			}

			return nil
		})
	}

	err = group.Wait()
	if err != nil {
		getLogger(ctx).WithError(err).Error("error get pieces")
		return nil, status.Error(codes.Internal, "error in get pieces")
	}

	isAllFileDownload := true
	for _, piece := range piecesMap {
		if !isPieceDownload[piece.SerialNumber] {
			getLogger(ctx).WithField("piece_position", piece.SerialNumber).Error("file not downloaded")
			isAllFileDownload = false
		}
	}

	file := &file{
		name:      info.Name,
		hash:      hashStr,
		allPieces: isAllFileDownload,
		length:    info.Length,
		piecesLen: info.PieceLength,
		piecesMap: piecesMap,
	}

	err = file.MergePieces(ctx)
	if err != nil {
		getLogger(ctx).Error("cannot merge file")
		return nil, status.Error(codes.Internal, err.Error())
	}

	p.hashFiles[file.hash] = file
	p.haveFiles[file.name] = file

	return &api.DownloadFileResponse{
		FilePath: getDownloadFilename(file.name),
	}, nil
}

// пришел запрос "дай кусок"
func (p *Peer) GetPiece(ctx context.Context, request *api.GetPieceRequest) (*api.Piece, error) {
	log := getLogger(ctx)

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
