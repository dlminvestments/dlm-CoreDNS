package torrent

import (
	"crypto/sha1"
	"io"
	"os"
	"strings"

	"github.com/zeebo/bencode"
)

const pieceLength = 2048 * 10

// pieces will hash the file in path on 256kb boundaries and return the (sha1) hashes.
func pieces(path string) (int, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	hashes := "" // concatenated string of hash (strings)
	buf := make([]byte, 2048)
	h := sha1.New()
	chunk := 0
	length := 0
	n, err := f.Read(buf)
	for err != nil {
		h.Write(buf[:n])
		chunk++
		length += n
		if chunk > 10 {
			chunk = 0
			hashes += string(h.Sum(nil))
			h = sha1.New()
		}
		n, err = f.Read(buf)
	}
	if n > 0 {
		length += n
		h.Write(buf[:n])
		hashes += string(h.Sum(nil))
	}

	return length, hashes, nil
}

// Info is the torrent meta data for a single file.
type Info struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
}

// TorrentInfo contains the meta data for this torrent.
type TorrentInfo struct {
	Nodes []string `bencode:"nodes"`
	Info  Info     `bencode:"info"`
}

func NewTorrentInfo(path string) (*TorrentInfo, error) {
	length, pieces, err := pieces(path)
	if err != nil {
		return nil, err
	}
	i := Info{Pieces: pieces, PieceLength: 2048 * 10, Length: length, Name: path}
	return &TorrentInfo{Nodes: []string{}, Info: i}, nil
}

func (t *TorrentInfo) ToReader() io.Reader {
	s, err := bencode.EncodeString(t)
	if err != nil {
		return nil
	}
	return strings.NewReader(s)
}
