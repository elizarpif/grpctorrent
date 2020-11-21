package main

import (
	"context"
	"errors"
	"github.com/elizarpif/grpctorrent/api"
	_ "github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type Peer struct {
	id uuid.UUID
	haveFiles map[string]*file

	tracker api.TrackerClient
}

func NewPeer(ctx context.Context, trackerAddr string) (*Peer, error) {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	trackerClient, err := grpc.DialContext(ctx, trackerAddr, opts...)
	if err != nil {
		return nil, err
	}

	return &Peer{
		id: uuid.New(),
		haveFiles: make(map[string]*file),
		tracker: api.NewTrackerClient(trackerClient),
	}, nil
}

// пришел запрос "дай кусок"
func (p *Peer) GetPiece(ctx context.Context, request *api.GetPieceRequest) (*api.Piece, error) {
	log := getLogger(ctx)

	file, exists := p.haveFiles[request.Hash]
	if !exists{
		log.Error("file doesn't exists")
		return nil, errors.New("file doesn't exists")
	}

	piece, exists := file.piecesMap[uint(request.SerialNumber)]
	if !exists{
		log.Error("piece doesn't exists")
		return nil, errors.New("piece doesn't exists")
	}

	return piece, nil
}