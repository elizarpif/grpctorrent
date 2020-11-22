package main

import (
	"context"
	"errors"
	"github.com/elizarpif/grpctorrent/api"
	"github.com/golang/protobuf/ptypes/empty"
	_ "github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"path"

	"google.golang.org/grpc"
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

	_, filename := path.Split(f.Name)

	file, err := newFile(filename)
	if err != nil {
		return nil, err
	}

	hash := file.hash

	p.hashFiles[hash] = file
	p.haveFiles[filename] = file

	_, err = p.tracker.Upload(ctx, &api.UploadFileRequest{
		ClientId:    p.id.String(),
		Name:        filename,
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

func (p *Peer) GetLocalFileInfo(ctx context.Context, f *api.File) (*api.FileInfo, error) {
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

	info, err := p.tracker.GetFileInfo(ctx, &api.DownloadFileRequest{Hash: getHash([]byte(f.Name))})
	if err != nil {
		getLogger(ctx).WithError(err).Error("cannot find file on server")
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return info, nil
}

// TODO use goroutines
// TODO add logger in middleware grpc
func (p *Peer) Download(ctx context.Context, f *api.DownloadFileRequest) (*api.FileInfo, error) {
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

	// пройтись по списку доступных пиров и скачать у них доступные файлы
	for anotherPeerAddr, positions := range peerAddrPositions {
		for _, position := range positions {
			// если такой кусок еще не загружен
			if !isPieceDownload[position] {

				opts := []grpc.DialOption{grpc.WithInsecure()}
				conn, err := grpc.DialContext(ctx, anotherPeerAddr, opts...)
				if err != nil {
					return nil, err
				}

				anotherPeer := api.NewPeerClient(conn)
				piece, err := anotherPeer.GetPiece(ctx, &api.GetPieceRequest{
					SerialNumber: position,
					Hash:         hashStr,
				})
				if err == nil {
					piecesMap[uint(position)] = piece
					isPieceDownload[position] = true

					_, err := p.tracker.PostPieceInfo(ctx, &api.PieceInfo{
						HashFile: hashStr,
						Serial:   position,
					})
					if err != nil {
						getLogger(ctx).WithError(err).Error("cannot post piece info")
					}
				} else {
					getLogger(ctx).WithError(err).WithField("remote_peer", anotherPeerAddr).Error("cannot get piece")
				}
			}
		}
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

	p.haveFiles[file.hash] = file

	return &api.FileInfo{
		Name:        info.Name,
		PieceLength: file.piecesLen,
		Pieces:      uint64(len(file.piecesMap)),
		Length:      file.length,
		Hash:        hashStr,
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
		log.Error("piece doesn't exists")
		return nil, errors.New("piece doesn't exists")
	}

	return piece, nil
}
