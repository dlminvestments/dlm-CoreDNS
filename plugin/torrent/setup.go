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
		return plugin.Error("sign", err)
	}

	c.OnStartup(func() error {
		// go tor.do()
		return nil
	})
	c.OnShutdown(func() error {
		close(tor.stop)
		return nil
	})

	// Don't call AddPlugin, *sign* is not a plugin.
	return nil
}

func parse(c *caddy.Controller) (*Torrent, error) {
	t := &Torrent{}
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
			case "seed":
				t.seed = true
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	return t, nil
}
