package traffic

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/traffic/xds"

	"github.com/caddyserver/caddy"
)

var log = clog.NewWithPlugin("traffic")

func init() { plugin.Register("traffic", setup) }

func setup(c *caddy.Controller) error {
	rand.Seed(int64(time.Now().Nanosecond()))
	t, err := parseTraffic(c)
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

func parseTraffic(c *caddy.Controller) (*Traffic, error) {
	node := "coredns"
	toHosts := []string{}
	var err error

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) < 1 {
			return nil, c.ArgErr()
		}
		toHosts, err = parse.HostPortOrFile(args...)
		if err != nil {
			return nil, err
		}
		for i := range toHosts {
			if !strings.HasPrefix(toHosts[i], transport.GRPC+"://") {
				return nil, fmt.Errorf("not a %s scheme: %s", transport.GRPC, toHosts[i])
			}
			// now cut the prefix off again, because the dialer needs to see normal address strings. All this
			// grpc:// stuff is to enfore uniformaty accross plugins and future proofing for other protocols.
			toHosts[i] = toHosts[i][len(transport.GRPC+"://"):]
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

	// TODO: only the first host is used.
	x, err := xds.New(toHosts[0], node)
	if err != nil {
		return nil, err
	}

	t := &Traffic{c: x}
	return t, nil
}
