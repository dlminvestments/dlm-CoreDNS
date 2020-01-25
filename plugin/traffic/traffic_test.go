package traffic

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/plugin/traffic/xds"

	xdspb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	corepb "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpointpb "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/miekg/dns"
	"google.golang.org/grpc"
)

func TestTraffic(t *testing.T) {
	c, err := xds.New("127.0.0.1:0", "test-id", grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	tr := &Traffic{c: c, origins: []string{"lb.example.org."}}

	tests := []struct {
		cla     *xdspb.ClusterLoadAssignment
		cluster string
		qtype   uint16
		rcode   int
		answer  string // address value of the A/AAAA record.
		ns      bool   // should there be a ns section.
	}{
		{
			cla:     &xdspb.ClusterLoadAssignment{},
			cluster: "web", qtype: dns.TypeA, rcode: dns.RcodeSuccess, ns: true,
		},
		{
			cla:     &xdspb.ClusterLoadAssignment{},
			cluster: "web", qtype: dns.TypeSRV, rcode: dns.RcodeSuccess, ns: true,
		},
		{
			cla:     &xdspb.ClusterLoadAssignment{},
			cluster: "does-not-exist", qtype: dns.TypeA, rcode: dns.RcodeNameError, ns: true,
		},
		// healthy endpoint
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints:   endpoints([]EndpointHealth{{"127.0.0.1", 18008, corepb.HealthStatus_HEALTHY}}),
			},
			cluster: "web", qtype: dns.TypeA, rcode: dns.RcodeSuccess, answer: "127.0.0.1",
		},
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints:   endpoints([]EndpointHealth{{"::1", 18008, corepb.HealthStatus_HEALTHY}}),
			},
			cluster: "web", qtype: dns.TypeAAAA, rcode: dns.RcodeSuccess, answer: "::1",
		},
		// unknown endpoint
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints:   endpoints([]EndpointHealth{{"127.0.0.1", 18008, corepb.HealthStatus_UNKNOWN}}),
			},
			cluster: "web", qtype: dns.TypeA, rcode: dns.RcodeSuccess, ns: true,
		},
		// unknown endpoint and healthy endpoint
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints: endpoints([]EndpointHealth{
					{"127.0.0.1", 18008, corepb.HealthStatus_UNKNOWN},
					{"127.0.0.2", 18008, corepb.HealthStatus_HEALTHY},
				}),
			},
			cluster: "web", qtype: dns.TypeA, rcode: dns.RcodeSuccess, answer: "127.0.0.2",
		},
		// SRV query healthy endpoint
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints: endpoints([]EndpointHealth{
					{"127.0.0.2", 18008, corepb.HealthStatus_HEALTHY},
				}),
			},
			cluster: "web", qtype: dns.TypeSRV, rcode: dns.RcodeSuccess, answer: "endpoint-0.web.lb.example.org.",
		},
		// A query for endpoint-0.
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints: endpoints([]EndpointHealth{
					{"127.0.0.2", 18008, corepb.HealthStatus_HEALTHY},
				}),
			},
			cluster: "endpoint-0.web", qtype: dns.TypeA, rcode: dns.RcodeSuccess, answer: "127.0.0.2",
		},
		// A query for endpoint-1.
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints: endpoints([]EndpointHealth{
					{"127.0.0.2", 18008, corepb.HealthStatus_HEALTHY},
					{"127.0.0.3", 18008, corepb.HealthStatus_HEALTHY},
				}),
			},
			cluster: "endpoint-1.web", qtype: dns.TypeA, rcode: dns.RcodeSuccess, answer: "127.0.0.3",
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		a := xds.NewAssignment()
		a.SetClusterLoadAssignment("web", tc.cla) // web is our cluster
		c.SetAssignments(a)

		m := new(dns.Msg)
		cl := dnsutil.Join(tc.cluster, tr.origins[0])
		m.SetQuestion(cl, tc.qtype)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := tr.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Errorf("Test %d: Expected no error, but got %q", i, err)
		}
		if rec.Msg.Rcode != tc.rcode {
			t.Errorf("Test %d: Expected no rcode %d, but got %d", i, tc.rcode, rec.Msg.Rcode)
		}
		if tc.ns && len(rec.Msg.Ns) == 0 {
			t.Errorf("Test %d: Expected authority section, but got none", i)
		}
		if tc.answer != "" && len(rec.Msg.Answer) == 0 {
			t.Fatalf("Test %d: Expected answer section, but got none", i)
		}
		if tc.answer != "" {
			record := rec.Msg.Answer[0]
			addr := ""
			switch x := record.(type) {
			case *dns.A:
				addr = x.A.String()
			case *dns.AAAA:
				addr = x.AAAA.String()
			case *dns.SRV:
				addr = x.Target
			}
			if tc.answer != addr {
				t.Errorf("Test %d: Expected answer %s, but got %s", i, tc.answer, addr)
			}
		}
	}
}

func TestTrafficLocality(t *testing.T) {
	c, err := xds.New("127.0.0.1:0", "test-id", grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	tr := &Traffic{c: c, origins: []string{"lb.example.org."}}

	tests := []struct {
		cla    *xdspb.ClusterLoadAssignment
		loc    xds.Locality // where we run
		answer string
	}{
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints: append(
					endpointsWithLocality([]EndpointHealth{
						{"127.0.0.1", 18008, corepb.HealthStatus_HEALTHY},
						{"127.0.0.2", 18008, corepb.HealthStatus_HEALTHY}},
						xds.Locality{Region: "us"}),
					endpointsWithLocality([]EndpointHealth{
						{"127.0.1.1", 18008, corepb.HealthStatus_HEALTHY}},
						xds.Locality{Region: "eu"})...,
				),
			},
			answer: "127.0.1.1",
			loc:    xds.Locality{Region: "eu"}, // our location
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		a := xds.NewAssignment()
		a.SetClusterLoadAssignment("web", tc.cla)
		c.SetAssignments(a)

		m := new(dns.Msg).SetQuestion(dnsutil.Join("web", tr.origins[0]), dns.TypeA)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := tr.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Errorf("Test %d: Expected no error, but got %q", i, err)
		}
		if x := rec.Msg.Answer[0].(*dns.A).A.String(); x != tc.answer {
			t.Fatalf("Test %d: Expected %s, but got %s", i, tc.answer, x)
		}
	}
}

func TestTrafficSRV(t *testing.T) {
	c, err := xds.New("127.0.0.1:0", "test-id", grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	tr := &Traffic{c: c, origins: []string{"lb.example.org."}}

	tests := []struct {
		cla     *xdspb.ClusterLoadAssignment
		cluster string
		qtype   uint16
		rcode   int
		answer  int // number of records in answer section
	}{
		// SRV query healthy endpoint
		{
			cla: &xdspb.ClusterLoadAssignment{
				ClusterName: "web",
				Endpoints: endpoints([]EndpointHealth{
					{"127.0.0.2", 18008, corepb.HealthStatus_HEALTHY},
					{"127.0.0.3", 18008, corepb.HealthStatus_HEALTHY},
				}),
			},
			cluster: "web", qtype: dns.TypeSRV, rcode: dns.RcodeSuccess, answer: 2,
		},
	}

	ctx := context.TODO()

	for i, tc := range tests {
		a := xds.NewAssignment()
		a.SetClusterLoadAssignment("web", tc.cla) // web is our cluster
		c.SetAssignments(a)

		m := new(dns.Msg)
		cl := dnsutil.Join(tc.cluster, tr.origins[0])
		m.SetQuestion(cl, tc.qtype)

		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		_, err := tr.ServeDNS(ctx, rec, m)
		if err != nil {
			t.Errorf("Test %d: Expected no error, but got %q", i, err)
		}
		if rec.Msg.Rcode != tc.rcode {
			t.Errorf("Test %d: Expected no rcode %d, but got %d", i, tc.rcode, rec.Msg.Rcode)
		}
		if tc.answer != len(rec.Msg.Answer) {
			t.Fatalf("Test %d: Expected %d answers, but got %d", i, tc.answer, len(rec.Msg.Answer))
		}
	}
}

type EndpointHealth struct {
	Address string
	Port    uint16
	Health  corepb.HealthStatus
}

func endpoints(e []EndpointHealth) []*endpointpb.LocalityLbEndpoints {
	return endpointsWithLocality(e, xds.Locality{})
}

func endpointsWithLocality(e []EndpointHealth, loc xds.Locality) []*endpointpb.LocalityLbEndpoints {
	ep := make([]*endpointpb.LocalityLbEndpoints, len(e))
	for i := range e {
		ep[i] = &endpointpb.LocalityLbEndpoints{
			Locality: &corepb.Locality{
				Region:  loc.Region,
				Zone:    loc.Zone,
				SubZone: loc.SubZone,
			},
			LbEndpoints: []*endpointpb.LbEndpoint{{
				HostIdentifier: &endpointpb.LbEndpoint_Endpoint{
					Endpoint: &endpointpb.Endpoint{
						Address: &corepb.Address{
							Address: &corepb.Address_SocketAddress{
								SocketAddress: &corepb.SocketAddress{
									Address: e[i].Address,
									PortSpecifier: &corepb.SocketAddress_PortValue{
										PortValue: uint32(e[i].Port),
									},
								},
							},
						},
					},
				},
				HealthStatus: e[i].Health,
			}},
		}
	}
	return ep
}
