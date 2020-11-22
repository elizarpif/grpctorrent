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
	files map[string]*availableFile // мапа хэш - колиечство доступных кусков
}

type availableFile struct {
	hash   string        // хэш файла
	pieces map[uint]bool // доступные куски для скачивания 
}

func newAvailableFile(hash string, pieceNum uint) *availableFile {
	pieces := make(map[uint]bool)
	pieces[pieceNum] = true

	return &availableFile{hash: hash, pieces: pieces}
}

func NewPeer(addr string, id uuid.UUID) *Peer {
	return &Peer{addr: addr, id: id, files: make(map[string]*availableFile)}
}

type Server struct {
	hashPeers map[string][]*Peer       // хэш файла к пирам
	hashFiles map[string]*api.FileInfo // хэш файла к файлу
	peers     map[string]*Peer         // пиры по address
}

func NewServer() *Server {
	return &Server{
		hashPeers: make(map[string][]*Peer),
		hashFiles: make(map[string]*api.FileInfo),
		peers:     make(map[string]*Peer),
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
	addr, err := getPeerAddrFromMetadata(ctx)
	if err != nil {
		getLogger(ctx).WithError(err).Error("cannot get peer from context")
		return nil, errors.New("cannot get peer from context")
	}

	isPeer := NewPeer(addr, clientID)
	s.peers[addr] = isPeer // добавляем в мапу пиров

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

	newPieceInfo := &availableFile{
		hash: file.Hash,
		pieces: make(map[uint]bool),
	}

	for i := 0; i < int(file.Pieces); i++ {
		newPieceInfo.pieces[uint(i)] = true
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

	addr, err := getPeerAddrFromMetadata(ctx)
	if err != nil {
		getLogger(ctx).WithError(err).Error("cannot get peer from context")
		return nil, errors.New("cannot get peer from context")
	}

	s.peers[addr] = NewPeer(addr, peerID)

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

		for k := range is.pieces {
			respPeer.SerialPieces = append(respPeer.SerialPieces, uint64(k))
		}

		resp.Peers = append(resp.Peers, respPeer)
	}

	return resp, nil
}

func (s *Server) PostPieceInfo(ctx context.Context, info *api.PieceInfo) (*empty.Empty, error) {
	addr, err := getPeerAddrFromMetadata(ctx)
	if err != nil {
		return nil, err
	}

	// получаем список пиров по хешу файла
	peers, ok := s.hashPeers[info.HashFile]
	if !ok {
		return nil, errors.New("hash doesnt exists")
	}

	// ищем текущего пира в списке пиров по хешу
	peer := findPeer(peers, addr)

	// если его нет...
	if peer == nil {
		// получаем текущего пира из мапы всех пиров
		currentPeer, ok := s.peers[addr]
		if !ok {
			return nil, errors.New("peer doesnt exists")
		}

		currentPeer.files[info.HashFile] = newAvailableFile(info.HashFile, uint(info.Serial))
		s.hashPeers[info.HashFile] = append(s.hashPeers[info.HashFile], currentPeer)
	} else {
		peer.files[info.HashFile] = newAvailableFile(info.HashFile, uint(info.Serial))
	}

	// check is file piece added

	return &empty.Empty{}, nil
}

func findPeer(peers []*Peer, addr string) *Peer {
	for _, p := range peers {
		if p.addr == addr {
			return p
		}
	}

	return nil
}

func (s *Server) PostFileInfo(ctx context.Context, download *api.AllPiecesDownload) (*empty.Empty, error) {
	panic("implement me")
}
