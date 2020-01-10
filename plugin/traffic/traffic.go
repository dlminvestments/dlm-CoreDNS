package traffic

import (
	"context"
	"math/rand"
	"sync"
	"time"

	clog "github.com/coredns/coredns/pkg/log"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/response"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("traffic")

// Traffic is a plugin that load balances according to assignments.
type Traffic struct {
	assignments map[string]assignment // zone -> assignment
	mu          sync.RWMutex          // protects assignments
	Next        plugin.Handler
}

// ServeDNS implements the plugin.Handler interface.
func (t *Traffic) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	tw := &ResponseWriter{ResponseWriter: w}
	t.mu.RLock()
	a, ok := t.assignments[state.Name()]
	t.mu.RUnlock()
	if ok {
		tw.a = &a
	}
	return plugin.NextOrFailure(t.Name(), t.Next, ctx, tw, r)
}

// Name implements the plugin.Handler interface.
func (t *Traffic) Name() string { return "traffic" }

// ResponseWriter writes a traffic load balanced response.
type ResponseWriter struct {
	dns.ResponseWriter
	a *assignment
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

	// ok, traffic-lb
	if r.a != nil {

	}
	if len(res.Answer) > 1 {
		res.Answer = []dns.RR{res.Answer[rand.Intn(len(res.Answer))]}
		res.Answer[0].Header().Ttl = 5
	}
	res.Ns = []dns.RR{} // remove auth section, we don't care

	return r.ResponseWriter.WriteMsg(res)
}
