package traffic

import (
	"context"
	"strings"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/traffic/xds"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

// Traffic is a plugin that load balances according to assignments.
type Traffic struct {
	c       *xds.Client
	id      string
	origins []string

	Next plugin.Handler
}

// ServeDNS implements the plugin.Handler interface.
func (t *Traffic) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{Req: r, W: w}

	cluster := ""
	for _, o := range t.origins {
		println(o, state.Name())
		if strings.HasSuffix(state.Name(), o) {
			cluster, _ = dnsutil.TrimZone(state.Name(), o)
			state.Zone = o
			break
		}
	}
	if cluster == "" {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	addr := t.c.Select(cluster)
	if addr != nil {
		log.Debugf("Found endpoint %q for %q", addr, cluster)
	} else {
		log.Debugf("No healthy endpoints found for %q", cluster)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	if addr == nil {
		m.Ns = soa(state.Zone)
		w.WriteMsg(m)
		return 0, nil
	}

	switch state.QType() {
	case dns.TypeA:
		if addr.To4() == nil { // it's an IPv6 address, return nodata in that case.
			m.Ns = soa(state.Zone)
			break
		}
		m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5}, A: addr}}

	case dns.TypeAAAA:
		if addr.To4() != nil { // it's an IPv4 address, return nodata in that case.
			m.Ns = soa(state.Zone)
			break
		}
		m.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5}, AAAA: addr}}
	default:
		m.Ns = soa(state.Zone)
	}

	w.WriteMsg(m)
	return 0, nil
}

// soa returns a synthetic so for this zone.
func soa(z string) []dns.RR {
	return []dns.RR{&dns.SOA{
		Hdr:     dns.RR_Header{Name: z, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 5},
		Ns:      dnsutil.Join("ns", z),
		Mbox:    dnsutil.Join("coredns", z),
		Serial:  uint32(time.Now().UTC().Unix()),
		Refresh: 14400,
		Retry:   3600,
		Expire:  604800,
		Minttl:  5,
	}}
}

// Name implements the plugin.Handler interface.
func (t *Traffic) Name() string { return "traffic" }
