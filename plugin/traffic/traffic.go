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
		if strings.HasSuffix(state.Name(), o) {
			cluster, _ = dnsutil.TrimZone(state.Name(), o)
			state.Zone = o
			break
		}
	}
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	addr, port, ok := t.c.Select(cluster)
	if !ok {
		// ok the cluster (which has potentially extra labels), doesn't exist, but we may have a query for endpoint-0.<cluster>.
		// check if we have 2 labels and that the first equals endpoint-0.
		if dns.CountLabel(cluster) != 2 {
			m.Ns = soa(state.Zone)
			m.Rcode = dns.RcodeNameError
			w.WriteMsg(m)
			return 0, nil
		}
		labels := dns.SplitDomainName(cluster)
		if strings.Compare(labels[0], "endpoint-0") == 0 {
			// recheck if the cluster exist.
			addr, port, ok = t.c.Select(labels[1])
			if !ok {
				m.Ns = soa(state.Zone)
				m.Rcode = dns.RcodeNameError
				w.WriteMsg(m)
				return 0, nil
			}
		}
	}

	if addr == nil {
		log.Debugf("No (healthy) endpoints found for %q", cluster)
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
		m.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 5}, AAAA: addr}}
	case dns.TypeSRV:
		target := dnsutil.Join("endpoint-0", cluster) + state.Zone
		m.Answer = []dns.RR{&dns.SRV{Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5},
			Priority: 100, Weight: 100, Port: port, Target: target}}
		if addr.To4() == nil {
			m.Extra = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: target, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5}, AAAA: addr}}
		} else {
			m.Extra = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: target, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5}, A: addr}}
		}
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
