package traffic

import (
	"math/rand"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/traffic/xds"

	"github.com/caddyserver/caddy"
)

var log = clog.NewWithPlugin("traffic")

func init() { plugin.Register("traffic", setup) }

func setup(c *caddy.Controller) error {
	rand.Seed(int64(time.Now().Nanosecond()))
	if err := parse(c); err != nil {
		return plugin.Error("traffic", err)
	}

	t, err := New()
	if err != nil {
		return plugin.Error("traffic", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		t.Next = next
		return t
	})

	t.c.WatchCluster("xds_experimental", func(xds.CDSUpdate, error) {})

	return nil
}

func parse(c *caddy.Controller) error {
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) != 0 {
			return c.ArgErr()

		}
	}
	return nil
}
