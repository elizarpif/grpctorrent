//nolint:gosec // for hash md5 can be weakcd gr
package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path"
	"sort"

	"github.com/elizarpif/grpctorrent/api"
	"github.com/elizarpif/logger"
)

type file struct {
	name      string
	hash      string
	allPieces bool
	length    uint64

	piecesLen uint64
	piecesMap map[uint]*api.Piece
}

// fixme
// установление длины каждого куска файла
func getPieceLength(length int) int {
	if length <= 256 {
		return 1
	}
	if length <= 1024 { // 1 KB
		return 256 // 256 B
	}
	if length <= 1024*1024 { // 1MB
		return 1024 // 1 KB
	}
	if length <= 256*1024*1024 { // 256 MB
		return 1024 * 1024 // 1MB
	}

	return 256 * 1024 * 1024 // 256 MB
}

// деление файла на куски
func splitFile(content []byte) (res map[uint]*api.Piece, length uint64) {
	pieceLen := getPieceLength(len(content))

	var serial uint64 = 0

	mapPiece := make(map[uint]*api.Piece)

	for i := 0; i < len(content); i += pieceLen {
		bound := i + pieceLen
		if len(content) < bound {
			bound = len(content)
		}

		mapPiece[uint(serial)] = &api.Piece{
			Payload:      content[i:bound],
			SerialNumber: serial,
		}

		serial++
	}

	return mapPiece, uint64(pieceLen)
}

func getHash(fContent []byte) string {
	hash := md5.Sum(fContent)
	return hex.EncodeToString(hash[:])
}

//nolint:gosec // for hash
// чтение файла и создание торрент-файла с последующей загрузкой
func newFile(name string) (*file, error) {
	fContent, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}

	_, filename := path.Split(name)

	pMap, pLen := splitFile(fContent)
	f := &file{
		length:    uint64(len(fContent)),
		name:      filename,
		hash:      getHash(fContent),
		piecesMap: pMap,
		piecesLen: pLen,
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

func getDownloadFilename(name string) string {
	dir := "/home/space/5 sem/networks/grpctorrent/peer"

	return path.Join(dir, "downloaded", name)
}

// запись в файл
func (f *file) write(bytes []byte) error {
	file, err := os.Create(getDownloadFilename(f.name))
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(bytes)
	return err
}

//nolint:gosec // for hash
// склеивание файла из кусочков
func (f *file) MergePieces(ctx context.Context) error {
	log := logger.GetLogger(ctx)

	if !f.allPieces {
		log.Warning("no all pieces")
		return nil
	}

	pieces := f.sortPieces()

	bytes := []byte{}
	for _, v := range pieces {
		bytes = append(bytes, v.Payload...)
	}

	tmp := md5.Sum(bytes)
	newHash := hex.EncodeToString(tmp[:])

	if f.hash != newHash {
		log.WithField("oldHash", f.hash).
			WithField("newHash", newHash).
			Error("hash not expected")
	}

	err := f.write(bytes)
	if err != nil {
		log.WithError(err).Error("can't write to file")
		return err
	}

	return nil
}
