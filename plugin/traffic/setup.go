package traffic

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/parse"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/coredns/coredns/plugin/traffic/xds"

	"github.com/caddyserver/caddy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	c.OnStartup(func() error {
		go t.c.Run()
		metrics.MustRegister(c, xds.ClusterGauge)
		return nil
	})
	c.OnShutdown(func() error { return t.c.Stop() })
	return nil
}

func parseTraffic(c *caddy.Controller) (*Traffic, error) {
	node := "coredns"
	toHosts := []string{}
	t := &Traffic{}
	var (
		err           error
		tlsConfig     *tls.Config
		tlsServerName string
	)

	t.origins = make([]string, len(c.ServerBlockKeys))
	for i := range c.ServerBlockKeys {
		t.origins[i] = plugin.Host(c.ServerBlockKeys[i]).Normalize()
	}

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
			// now cut the prefix off again, because the dialler needs to see normal address strings. All this
			// grpc:// stuff is to enforce uniform across plugins and future proofing for other protocols.
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
			case "tls":
				args := c.RemainingArgs()
				if len(args) > 3 {
					return nil, c.ArgErr()
				}

				tlsConfig, err = pkgtls.NewTLSConfigFromArgs(args...)
				if err != nil {
					return nil, err
				}
			case "tls_servername":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				tlsServerName = c.Val()
			case "ignore_health":
				t.health = true
			default:
				return nil, c.Errf("unknown property '%s'", c.Val())
			}
		}
	}

	opts := []grpc.DialOption{grpc.WithInsecure()}
	if tlsConfig != nil {
		if tlsServerName != "" {
			tlsConfig.ServerName = tlsServerName
		}
		opts = []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))}
	}

	// TODO: only the first host is used, need to figure out how to reconcile multiple upstream providers.
	if t.c, err = xds.New(toHosts[0], node, opts...); err != nil {
		return nil, err
	}

	return t, nil
}

// parseLocality parses string s into loc's. Each loc must be space separated from the other, inside
// a loc we have region,zone,subzone, where subzone or subzone and zone maybe empty. If specified
// they must be comma separate (not spaces or anything).
func parseLocality(s string) ([]xds.Locality, error) {
	sets := strings.Fields(s)
	if len(sets) == 0 {
		return nil, nil
	}

	locs := []xds.Locality{}
	for _, s := range sets {
		l := strings.Split(s, ",")
		switch len(l) {
		default:
			return nil, fmt.Errorf("too many location specifiers: %q", s)
		case 1:
			locs = append(locs, xds.Locality{Region: l[0]})
			continue
		case 2:
			locs = append(locs, xds.Locality{Region: l[0], Zone: l[1]})
			continue
		case 3:
			locs = append(locs, xds.Locality{Region: l[0], Zone: l[1], SubZone: l[2]})
			continue
		}

	}
	return locs, nil
}
