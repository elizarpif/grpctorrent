package main

import (
	"context"
	"github.com/google/uuid"

	"github.com/elizarpif/grpctorrent/api"
	_ "github.com/golang/protobuf/ptypes/empty"
)

type Peer struct {
	id uuid.UUID
	haveFiles map[string]*file
}

func NewPeer() *Peer {
	return &Peer{id: uuid.New()}
}

// пришел запрос "дай кусок"
func (p *Peer) GetPiece(ctx context.Context, request *api.GetPieceRequest) (*api.Piece, error) {
	
}

