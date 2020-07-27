package torrent

import (
	"path/filepath"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/caddyserver/caddy"
)

func init() { plugin.Register("torrent", setup) }

func setup(c *caddy.Controller) error {
	tor, err := parse(c)
	if err != nil {
		return plugin.Error("torrent", err)
	}

	c.OnStartup(func() error {
		err := tor.Do()
		return err
	})
	c.OnShutdown(func() error {
		close(tor.stop)
		return nil
	})

	// Don't call AddPlugin, *sign* is not a plugin.
	return nil
}

func parse(c *caddy.Controller) (*Torrent, error) {
	t := &Torrent{stop: make(chan struct{})}
	config := dnsserver.GetConfig(c)

	for c.Next() {
		if !c.NextArg() {
			return nil, c.ArgErr()
		}
		dbfile := c.Val()
		if !filepath.IsAbs(dbfile) && config.Root != "" {
			dbfile = filepath.Join(config.Root, dbfile)
		}
		t.dbfile = dbfile

		for c.NextBlock() {
			switch c.Val() {
			case "dht":
				t.dht = true
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	return t, nil
}
