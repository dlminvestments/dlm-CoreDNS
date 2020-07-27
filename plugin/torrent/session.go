package torrent

import (
	"log"
	"time"

	rtorrent "github.com/cenkalti/rain/torrent"
)

func (t *Torrent) StartSession() error {
	s, err := rtorrent.NewSession(torrent.DefaultConfig)
	if err != nil {
		return err
	}

	// Add magnet link
	tor, _ := ses.AddURI(magnetLink, nil)

	// Watch the progress
	for range time.Tick(time.Second) {
		s := tor.Stats()
		log.Printf("Status: %s, Downloaded: %d, Peers: %d", s.Status.String(), s.Bytes.Completed, s.Peers.Total)
	}

}
