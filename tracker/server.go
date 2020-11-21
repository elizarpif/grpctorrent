package main

import (
	"context"
	"errors"
	"net"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"
	"google.golang.org/grpc/peer"
)

type Peer struct {
	addr  net.Addr
	id    uuid.UUID
	files map[string]*api.PiecesInfo
}

func NewPeer(addr net.Addr, id uuid.UUID) *Peer {
	return &Peer{addr: addr, id: id, files: make(map[string]*api.PiecesInfo)}
}

type Server struct {
	hashPeers map[string][]*Peer
	peers     map[uuid.UUID]*Peer
}

func NewServer() *Server {
	return &Server{
		hashPeers: make(map[string][]*Peer),
		peers:     make(map[uuid.UUID]*Peer),
	}
}

func (s *Server) Upload(ctx context.Context, file *api.TorrentFile) (*empty.Empty, error) {
	panic("implement me")
}

func (s *Server) GetPeers(ctx context.Context, request *api.GetPeersRequest) (*api.ListPeers, error) {
	peerID := uuid.MustParse(request.PeerId)

	// если такого пира не существует, то добавим в список
	if _, exists := s.peers[peerID]; !exists {
		p, ok := peer.FromContext(ctx)
		if !ok {
			getLogger(ctx).Error("cannot get peer from context")
			return nil, errors.New("cannot get peer from context")
		}

		s.peers[peerID] = NewPeer(p.Addr, peerID)
	}

	resp := &api.ListPeers{}

	peers := s.hashPeers[request.HashFile]
	for _, p := range peers {
		respPeer := &api.ListPeers_Peer{
			Address: p.addr.String(),
		}

		is, ok := p.files[request.HashFile]
		if !ok {
			getLogger(ctx).Error("files in peer doesnt exist!")
			return nil, errors.New("files in peer doesnt exist!")
		}

		respPeer.SerialPieces = is.SerialPieces
		resp.Peers = append(resp.Peers, respPeer)
	}

	return resp, nil
}

func (s *Server) PostPiecesInfo(ctx context.Context, info *api.PiecesInfo) (*empty.Empty, error) {
	panic("implement me")
}
