package torrent

// Torrent contains the file data that needs to be torrented.
type Torrent struct {
	dbfile string
	dht    bool

	stop chan struct{}
}
