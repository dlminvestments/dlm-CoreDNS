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
		go func() {
			for {
				opts := []grpc.DialOption{grpc.WithInsecure()}
				if t.tlsConfig != nil {
					opts = []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(t.tlsConfig))}
				}

				i := 0
			redo:
				i = i % len(t.hosts)
				if t.c, err = xds.New(t.hosts[i], t.node, opts...); err != nil {
					log.Warning(err)
					time.Sleep(2 * time.Second) // back off foo
					i++
					goto redo
				}

				if err := t.c.Run(); err != nil {
					log.Warning(err)
					time.Sleep(2 * time.Second) // back off foo
					i++
					goto redo
				}
				// err == nil, we are connected
				break
			}
		}()
		metrics.MustRegister(c, xds.ClusterGauge)
		return nil
	})
	c.OnShutdown(func() error { return t.c.Stop() })
	return nil
}

func parseTraffic(c *caddy.Controller) (*Traffic, error) {
	toHosts := []string{}
	t := &Traffic{node: "coredns", mgmt: "xds"}
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
			case "cluster":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				t.mgmt = args[0]
			case "id":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				t.node = args[0]
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

	if tlsConfig != nil {
		t.tlsConfig = tlsConfig
		if tlsServerName != "" {
			t.tlsConfig.ServerName = tlsServerName
		}
	}
	t.hosts = toHosts
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
			l0 := strings.TrimSpace(l[0])
			if l0 == "" {
				return nil, fmt.Errorf("empty location specifer: %q", l[0])
			}
			locs = append(locs, xds.Locality{Region: l0})
			continue
		case 2:
			l0 := strings.TrimSpace(l[0])
			if l0 == "" {
				return nil, fmt.Errorf("empty location specifer: %q", l[0])
			}
			l1 := strings.TrimSpace(l[1])
			if l1 == "" {
				return nil, fmt.Errorf("empty location specifer: %q", l[1])
			}
			locs = append(locs, xds.Locality{Region: l0, Zone: l1})
			continue
		case 3:
			l0 := strings.TrimSpace(l[0])
			if l0 == "" {
				return nil, fmt.Errorf("empty location specifer: %q", l[0])
			}
			l1 := strings.TrimSpace(l[1])
			if l1 == "" {
				return nil, fmt.Errorf("empty location specifer: %q", l[1])
			}
			l2 := strings.TrimSpace(l[2])
			if l2 == "" {
				return nil, fmt.Errorf("empty location specifer: %q", l[2])
			}
			locs = append(locs, xds.Locality{Region: l0, Zone: l1, SubZone: l2})
			continue
		}

	}
	return locs, nil
}
