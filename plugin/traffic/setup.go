package traffic

import (
	"math/rand"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/caddyserver/caddy"
)

var log = clog.NewWithPlugin("traffic")

func init() { plugin.Register("traffic", setup) }

func setup(c *caddy.Controller) error {
	rand.Seed(int64(time.Now().Nanosecond()))
	if err := parse(c); err != nil {
		return plugin.Error("traffic", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &Traffic{Next: next, assignments: make(map[string]assignment)}
	})

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
