package traffic

import (
	"fmt"

	"github.com/miekg/dns"
)

// See https://github.com/grpc/grpc/blob/master/doc/service_config.md for the fields in this proto.
// We encode it as json and return it in a TXT field.
// TOOD(miek): balancer name should not be hardcoded
var lbTXT = `grpc_config=[{"serviceConfig":{"loadBalancingConfig":[{"eds_experimental":{"Cluster": "xds", "EDSServiceName":"%s", "BalancerName":"xds"}}]}}]`

func txt(z, cluster string) []dns.RR {
	return []dns.RR{&dns.TXT{
		Hdr: dns.RR_Header{Name: z, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 5},
		Txt: []string{fmt.Sprintf(lbTXT, cluster)},
	}}
}
