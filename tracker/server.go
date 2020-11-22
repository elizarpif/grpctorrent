package main

import (
	"context"
	"errors"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"

	"google.golang.org/grpc/metadata"
)

type Peer struct {
	addr  string
	id    uuid.UUID
	files map[string]*api.PiecesInfo
}

func NewPeer(addr string, id uuid.UUID) *Peer {
	return &Peer{addr: addr, id: id, files: make(map[string]*api.PiecesInfo)}
}

type Server struct {
	hashPeers map[string][]*Peer // хэш файла к пирам
	hashFiles map[string]*api.FileInfo
	peers     map[uuid.UUID]*Peer // пиры по id
}

func NewServer() *Server {
	return &Server{
		hashPeers: make(map[string][]*Peer),
		hashFiles: make(map[string]*api.FileInfo),
		peers:     make(map[uuid.UUID]*Peer),
	}
}

func (s *Server) GetFileInfo(ctx context.Context, file *api.DownloadFileRequest) (*api.FileInfo, error) {
	is, ok := s.hashFiles[file.Hash]
	if !ok {
		return nil, errors.New("cannot find file")
	}

	return is, nil
}

func getPeerAddrFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("missing metadata")
	}

	addr, ok := md["address"]
	if !ok {
		return "", errors.New("invalid metadata")
	}

	return addr[0], nil
}

func (s *Server) addPeer(ctx context.Context, clientID uuid.UUID) (*Peer, error) {
	isPeer, exists := s.peers[clientID]
	// если такого пира не существует, то добавим в список
	if !exists {
		addr, err := getPeerAddrFromMetadata(ctx)
		if err != nil {
			getLogger(ctx).WithError(err).Error("cannot get peer from context")
			return nil, errors.New("cannot get peer from context")
		}

		isPeer = NewPeer(addr, clientID)
		s.peers[clientID] = isPeer // добавляем в мапу пиров
	}

	return isPeer, nil
}

func (s *Server) Upload(ctx context.Context, file *api.UploadFileRequest) (*empty.Empty, error) {
	clientID := uuid.MustParse(file.ClientId)

	// добавляем пира в список
	isPeer, err := s.addPeer(ctx, clientID)
	if err != nil {
		return nil, err
	}

	getLogger(ctx)
	// добавляем информацию о файле в мапу
	s.hashFiles[file.Hash] = &api.FileInfo{
		Name:        file.Name,
		PieceLength: file.PieceLength,
		Pieces:      file.Pieces,
		Length:      file.Length,
		Hash:        file.Hash,
	}

	newPieceInfo := &api.PiecesInfo{
		HashFile: file.Hash,
		AllFile:  true,
	}

	for i := 0; i < int(file.Pieces); i++ {
		newPieceInfo.SerialPieces = append(newPieceInfo.SerialPieces, uint64(i))
	}

	// добавляем информацию о загруженном файле к пиру
	isPeer.files[file.Hash] = newPieceInfo

	// добавляем пира к мапе хэш-пиры
	peers := s.hashPeers[file.Hash]
	peers = append(peers, isPeer)
	s.hashPeers[file.Hash] = peers

	return &empty.Empty{}, nil
}

func (s *Server) GetPeers(ctx context.Context, request *api.GetPeersRequest) (*api.ListPeers, error) {
	peerID := uuid.MustParse(request.PeerId)

	// если такого пира не существует, то добавим в список
	if _, exists := s.peers[peerID]; !exists {
		addr, err := getPeerAddrFromMetadata(ctx)
		if err != nil{
			getLogger(ctx).WithError(err).Error("cannot get peer from context")
			return nil, errors.New("cannot get peer from context")
		}

		s.peers[peerID] = NewPeer(addr, peerID)
	}

	resp := &api.ListPeers{}

	peers := s.hashPeers[request.HashFile]
	for _, p := range peers {
		respPeer := &api.ListPeers_Peer{
			Address: p.addr,
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
