package main

import (
	"context"
	"crypto/md5"
	"errors"
	"io/ioutil"
	"os"
	"sort"

	"github.com/elizarpif/grpctorrent/api"
)

type file struct {
	name      string
	hash      [16]byte
	allPieces bool

	piecesMap map[uint]*api.Piece
}

// fixme
// установление длины каждого куска файла
func getPieceLength(length int) int {
	if length < 256 {
		return 1
	}
	if length < 1024 { // 1 KB
		return 256 // 256 B
	}
	if length < 1024*1024 { // 1MB
		return 256 * 1024 // 256KB
	}
	if length < 256*1024*1024 { // 256 MB
		return 1024 * 1024 // 1MB
	}

	return 256 * 1024 * 1024 // 256 MB
}

// деление файла на куски
func splitFile(content []byte) map[uint]*api.Piece {
	pieceLen := getPieceLength(len(content))

	var serial uint64 = 0

	mapPiece := make(map[uint]*api.Piece)

	for i := 0; i < len(content); i += pieceLen {
		mapPiece[uint(i)] = &api.Piece{
			Payload:      string(content[i:pieceLen]),
			SerialNumber: serial,
		}

		serial++
	}

	return mapPiece
}

// чтение файла и создание торрент-файла с последующей загрузкой
func newFile(name string) (*file, error) {
	fContent, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}

	f := &file{
		name:      name,
		hash:      md5.Sum(fContent),
		piecesMap: splitFile(fContent),
		allPieces: true,
	}

	return f, nil
}

// сортировка кусочков (на всякий)
func (f *file) sortPieces() []*api.Piece {
	pieces := make([]*api.Piece, 0, len(f.piecesMap))
	for _, v := range f.piecesMap {
		pieces = append(pieces, v)
	}

	sort.SliceStable(pieces, func(i, j int) bool {
		return pieces[i].SerialNumber < pieces[j].SerialNumber
	})

	return pieces
}

// запись в файл
func (f *file) write(bytes []byte) error {
	file, err := os.Create(f.name)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(bytes)
	return err
}

// склеивание файла из кусочков
func (f *file) MergePieces(ctx context.Context) error {
	log := getLogger(ctx)

	if !f.allPieces {
		log.Warning("no all pieces")
		return nil
	}

	pieces := f.sortPieces()

	bytes := []byte{}
	for _, v := range pieces {
		bytes = append(bytes, []byte(v.Payload)...)
	}

	newHash := md5.Sum(bytes)
	if f.hash != newHash {
		log.WithField("oldHash", f.hash).
			WithField("newHash", newHash).
			Error("hash not expected")
		return errors.New("hash not expected")
	}

	err := f.write(bytes)
	if err != nil {
		log.WithError(err).Error("can't write to file")
		return err
	}

	return nil
}