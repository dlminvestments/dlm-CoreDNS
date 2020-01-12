package traffic

import (
	"context"
	"math/rand"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/plugin/traffic/xds"
	"github.com/coredns/coredns/plugin/traffic/xds/bootstrap"

	"github.com/miekg/dns"
)

// Traffic is a plugin that load balances according to assignments.
type Traffic struct {
	c    *xds.Client
	Next plugin.Handler
}

// New returns a pointer to a new and initialized Traffic.
func New() (*Traffic, error) {
	config, err := bootstrap.NewConfig()
	if err != nil {
		return nil, err
	}
	c, err := xds.New(xds.Options{Config: *config})
	if err != nil {
		return nil, err
	}

	return &Traffic{c: c}, nil
}

func (t *Traffic) Close() {
	t.c.Close()
}

// ServeDNS implements the plugin.Handler interface.
func (t *Traffic) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	tw := &ResponseWriter{ResponseWriter: w}
	return plugin.NextOrFailure(t.Name(), t.Next, ctx, tw, r)
}

// Name implements the plugin.Handler interface.
func (t *Traffic) Name() string { return "traffic" }

// ResponseWriter writes a traffic load balanced response.
type ResponseWriter struct {
	dns.ResponseWriter
}

// WriteMsg implements the dns.ResponseWriter interface.
func (r *ResponseWriter) WriteMsg(res *dns.Msg) error {
	// set all TTLs to 5, also negative TTL?
	if res.Rcode != dns.RcodeSuccess {
		return r.ResponseWriter.WriteMsg(res)
	}

	if res.Question[0].Qtype != dns.TypeA && res.Question[0].Qtype != dns.TypeAAAA {
		return r.ResponseWriter.WriteMsg(res)
	}

	typ, _ := response.Typify(res, time.Now().UTC())
	if typ != response.NoError {
		return r.ResponseWriter.WriteMsg(res)
	}

	if len(res.Answer) > 1 {
		res.Answer = []dns.RR{res.Answer[rand.Intn(len(res.Answer))]}
		res.Answer[0].Header().Ttl = 5
	}
	res.Ns = []dns.RR{} // remove auth section, we don't care

	return r.ResponseWriter.WriteMsg(res)
}
