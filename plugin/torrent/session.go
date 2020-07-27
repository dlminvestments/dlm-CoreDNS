package torrent

import (
	"io/ioutil"
	"log"
	"path/filepath"
	"time"

	rtorrent "github.com/cenkalti/rain/torrent"
)

func (t *Torrent) Do() error {
	dc := rtorrent.DefaultConfig
	dc.DHTEnabled = t.dht
	dc.RPCEnabled = false
	dc.DHTBootstrapNodes = []string{"127.0.0.1:7246"} // its a me

	td, err := ioutil.TempDir("", "example")
	if err != nil {
		return err
	}
	dc.DataDir = td
	dc.Database = filepath.Join(td, "session.db")
	s, err := rtorrent.NewSession(dc)
	if err != nil {
		return err
	}

	ti, err := NewTorrentInfo("plugin/torrent/testdata/db.miek.nl")
	if err != nil {
		return err
	}

	tor, err := s.AddTorrent(ti.ToReader(), nil)
	if err != nil {
		return err
	}
	//	mag, _ := tor.Magnet()

	go s.StartAll()

	// Watch the progress
	for range time.Tick(time.Second) {
		s := tor.Stats()
		log.Printf("Status: %s, Downloaded: %d, Peers: %d", s.Status.String(), s.Bytes.Completed, s.Peers.Total)
	}
	return nil

}
