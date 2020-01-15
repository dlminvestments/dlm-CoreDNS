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

	t, err := New()
	if err != nil {
		return plugin.Error("traffic", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		t.Next = next
		return t
	})

	stream, err := t.c.Run()
	if err != nil {
		return plugin.Error("traffic", err)
	}

	if err := t.c.ClusterDiscovery(stream, "", "", []string{}); err != nil {
		log.Error(err)
	}

	err = t.c.Receive(stream)
	if err != nil {
		return plugin.Error("traffic", err)
	}

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
