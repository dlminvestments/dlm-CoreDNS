// generated by directives_generate.go; DO NOT EDIT

package dnsserver

// Directives are registered in the order they should be
// executed.
//
// Ordering is VERY important. Every plugin will
// feel the effects of all other plugin below
// (after) them during a request, but they must not
// care what plugin above them are doing.
var Directives = []string{
	"metadata",
	"geoip",
	"cancel",
	"tls",
	"reload",
	"nsid",
	"bufsize",
	"root",
	"bind",
	"debug",
	"ready",
	"health",
	"pprof",
	"prometheus",
	"errors",
	"log",
	"dnstap",
	"local",
	"dns64",
	"acl",
	"any",
	"chaos",
	"traffic",
	"loadbalance",
	"cache",
	"rewrite",
	"header",
	"dnssec",
	"autopath",
	"minimal",
	"template",
	"transfer",
	"hosts",
traffic
=======
	"route53",
	"azure",
	"clouddns",
	"k8s_external",
master
	"kubernetes",
	"file",
	"auto",
	"secondary",
	"etcd",
	"loop",
	"forward",
	"grpc",
	"erratic",
	"whoami",
	"on",
	"sign",
}
