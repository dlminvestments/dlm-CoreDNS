package traffic

import (
	"context"
	"fmt"
	"strconv"
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
	health  bool
	origins []string
	loc     []xds.Locality

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

	sockaddr, ok := t.c.Select(cluster, t.loc, t.health)
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
		if strings.HasPrefix(labels[0], "endpoint-") {
			// recheck if the cluster exist.
			cluster = labels[1]
			sockaddr, ok = t.c.Select(cluster, t.loc, t.health)
			if !ok {
				m.Ns = soa(state.Zone)
				m.Rcode = dns.RcodeNameError
				w.WriteMsg(m)
				return 0, nil
			}
			return t.serveEndpoint(ctx, state, labels[0], cluster)
		}
	}

	if sockaddr == nil {
		log.Debugf("No (healthy) endpoints found for %q", cluster)
		m.Ns = soa(state.Zone)
		w.WriteMsg(m)
		return 0, nil
	}

	switch state.QType() {
	case dns.TypeA:
		if sockaddr.Address().To4() == nil { // it's an IPv6 address, return nodata in that case.
			m.Ns = soa(state.Zone)
			break
		}
		m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5}, A: sockaddr.Address()}}

	case dns.TypeAAAA:
		if sockaddr.Address().To4() != nil { // it's an IPv4 address, return nodata in that case.
			m.Ns = soa(state.Zone)
			break
		}
		m.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 5}, AAAA: sockaddr.Address()}}
	case dns.TypeSRV:
		sockaddrs, _ := t.c.All(cluster, t.loc, t.health)
		m.Answer = make([]dns.RR, 0, len(sockaddrs))
		m.Extra = make([]dns.RR, 0, len(sockaddrs))
		for i, sa := range sockaddrs {
			target := fmt.Sprintf("endpoint-%d.%s.%s", i, cluster, state.Zone)

			m.Answer = append(m.Answer, &dns.SRV{
				Hdr:      dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 5},
				Priority: 100, Weight: 100, Port: sa.Port(), Target: target})

			if sa.Address().To4() == nil {
				m.Extra = append(m.Extra, &dns.AAAA{Hdr: dns.RR_Header{Name: target, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 5}, AAAA: sa.Address()})
			} else {
				m.Extra = append(m.Extra, &dns.A{Hdr: dns.RR_Header{Name: target, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 5}, A: sa.Address()})
			}
		}
	default:
		m.Ns = soa(state.Zone)
	}

	w.WriteMsg(m)
	return 0, nil
}

func (t *Traffic) serveEndpoint(ctx context.Context, state request.Request, endpoint, cluster string) (int, error) {
	m := new(dns.Msg)
	m.SetReply(state.Req)
	m.Authoritative = true

	// get endpoint number
	i := strings.Index(endpoint, "-")
	if i == -1 || i == len(endpoint) {
		m.Ns = soa(state.Zone)
		m.Rcode = dns.RcodeNameError
		state.W.WriteMsg(m)
		return 0, nil
	}

	end := endpoint[i+1:] // +1 to remove '-'
	nr, err := strconv.Atoi(end)
	if err != nil {
		m.Ns = soa(state.Zone)
		m.Rcode = dns.RcodeNameError
		state.W.WriteMsg(m)
		return 0, nil
	}

	sockaddrs, _ := t.c.All(cluster, t.loc, t.health)
	if len(sockaddrs) < nr {
		m.Ns = soa(state.Zone)
		m.Rcode = dns.RcodeNameError
		state.W.WriteMsg(m)
		return 0, nil
	}

	addr := sockaddrs[nr].Address()
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
	default:
		m.Ns = soa(state.Zone)
	}

	state.W.WriteMsg(m)
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
