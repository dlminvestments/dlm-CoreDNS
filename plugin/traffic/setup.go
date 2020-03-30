package traffic

import (
	"crypto/tls"
	"encoding/json"
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
	if _, err := json.Marshal(lbTXT); err != nil {
		return fmt.Errorf("failed to marshal grpc serverConfig: %s", err)
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
