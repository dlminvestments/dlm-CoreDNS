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
	t, err := parse(c)
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

	go func() {
		err = t.c.Receive(stream)
		if err != nil {
			// can't do log debug in setup functions
			log.Debug(err)
		}
	}()

	return nil
}

func parse(c *caddy.Controller) (*Traffic, error) {
	node := "coredns"

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) != 0 {
			return nil, c.ArgErr()

		}
		for c.NextBlock() {
			switch c.Val() {
			case "id":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				node = args[0]
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	x, err := xds.New(":18000", node)
	if err != nil {
		return nil, err
	}

	t := &Traffic{c: x}
	return t, nil
}
