package traffic

import "github.com/miekg/dns"

// See https://github.com/grpc/grpc/blob/master/doc/service_config.md for the fields in this proto.
// We encode it as json and return it in a TXT field.
var lbTXTxds = `grpc_config=[{"serviceConfig":{"loadBalancingConfig":[{"xds_experimental":{"lrs_load_reporting_server_name":""}}]}}]`

// Current impl. that will be removed in favor of xds
var lbTXTgrpc = `grpc_config=[{"serviceConfig":{"loadBalancingConfig":[{"grpclb":{}}]}}]`

func txt(z string) []dns.RR {
	return []dns.RR{&dns.TXT{
		Hdr: dns.RR_Header{Name: z, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 5},
		Txt: []string{lbTXTgrpc},
	}}
}
