# traffic

## Name

*traffic* - handout addresses according to assignments from Envoy's xDS.

## Description

The *traffic* plugin is a balancer that allows traffic steering, weighted responses and draining
of clusters. A cluster is defined as: "A group of logically similar endpoints that Envoy
connects to." Each cluster has a name, which *traffic* extends to be a domain name. See "Naming
Clusters" below.

The use case for this plugin is when a cluster has endpoints running in multiple (e.g. Kubernetes)
clusters and you need to steer traffic to (or away) from these endpoints, i.e. endpoint A needs to
be upgraded, so all traffic to it is drained. Or the entire Kubernetes needs to upgraded, and *all*
endpoints need to be drained from it.

The cluster information is retrieved from a service discovery manager that implements the service
discovery [protocols from Envoy](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol).
It connects to the manager using the Aggregated Discovery Service (ADS) protocol. Endpoints and
clusters are discovered every 10 seconds. The plugin hands out responses that adhere to these
assignments. Only endpoints that are *healthy* are handed out.

Note that the manager *itself* is also a cluster that is managed *by the management server*. This is
the *management cluster* (see `cluster` below in "Syntax"). By default the name for cluster is `xds`.
When bootstrapping *traffic* tries to retrieve the cluster endpoints for the management cluster,
when the cluster is not found *traffic* will return a fatal error.

The *traffic* plugin handles A, AAAA, SRV and TXT queries. TXT queries are purely used for debugging
as health status of the endpoints is ignored in that case.
Queries for non-existent clusters get a NXDOMAIN, where the minimal TTL is also set to 5s.

For A and AAAA queries each DNS response contains a single IP address that's considered the best
one. The TTL on these answer is set to 5s. It will only return successful responses either with an
answer or, otherwise, a NODATA response.

TXT replies will use the SRV record format augmented with the health status of each backend, as this
is useful for debugging.

~~~
web.lb.example.org.	5	IN	TXT	"100" "100" "18008" "endpoint-0.web.lb.example.org." "HEALTHY"
~~~

For SRV queries *all* healthy backends will be returned - assuming the client doing the query
is smart enough to select the best one. When SRV records are returned, the endpoint DNS names
are synthesized `endpoint-<N>.<cluster>.<zone>` that carries the IP address. Querying for these
synthesized names works as well.

*Traffic* implements version 2 of the xDS API. It works with the management server as written in
<https://github.com/miekg/xds>.

## Syntax

~~~
traffic TO...
~~~

This enabled the *traffic* plugin, with a default node ID of `coredns` and no TLS.

 *  **TO...** are the control plane endpoints to bootstrap from. These must start with `grpc://`. The
    port number defaults to 443, if not specified. These endpoints will be tried in the order given.

The extended syntax is available if you want more control.

~~~
traffic TO... {
    cluster CLUSTER
    id ID
    tls CERT KEY CA
    tls_servername NAME
}
~~~

 *  `cluster` **CLUSTER** define the name of the management cluster. By default this is `xds`.

 *  `id` **ID** is how *traffic* identifies itself to the control plane. This defaults to `coredns`.

 *  `tls` **CERT** **KEY** **CA** define the TLS properties for gRPC connection. If this is omitted
    an insecure connection is attempted. From 0 to 3 arguments can be provided with the meaning as
    described below

     -  `tls` - no client authentication is used, and the system CAs are used to verify the server
        certificate

     -  `tls` **CA** - no client authentication is used, and the file CA is used to verify the
        server certificate

     -  `tls` **CERT** **KEY** - client authentication is used with the specified cert/key pair. The
        server certificate is verified with the system CAs.

     -  `tls` **CERT** **KEY** **CA** - client authentication is used with the specified cert/key
        pair. The server certificate is verified using the specified CA file.

 *  `tls_servername` **NAME** allows you to set a server name in the TLS configuration. This is
    needed because *traffic* connects to an IP address, so it can't infer the server name from it.

## Naming Clusters

When a cluster is named this usually consists out of a single word, i.e. "cluster-v0", or "web".
The *traffic* plugins uses the name(s) specified in the Server Block to create fully qualified
domain names. For example if the Server Block specifies `lb.example.org` as one of the names,
and "cluster-v0" is one of the load balanced cluster, *traffic* will respond to queries asking for
`cluster-v0.lb.example.org.` and the same goes for "web"; `web.lb.example.org`.

For SRV queries all endpoints are returned, the SRV target names are synthesized:
`endpoint-<N>.web.lb.example.org` to take the example from above. *N* is an integer starting with 0.

## Matching Algorithm

How are queries match against the data we receive from xDS endpoint?

1.  Does the cluster exist? If not return NXDOMAIN, otherwise continue.

2.  Run through the endpoints, discard any endpoints that are not HEALTHY. If we are left with no
    endpoint return a NODATA response, otherwise continue.

3.  If weights are assigned, use those to pick an endpoint, otherwise randomly pick one and return a
    response to the client. Weights are copied from the xDS data, priority is not used and set to 0
    for all SRV records. Note that weights in SRV records are 16 bits, but xDS uses uint32; you have
    been warned.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

 *  `coredns_traffic_clusters_tracked{}` the number of tracked clusters.
 *  `coredns_traffic_endpoints_tracked{}` the number of tracked endpoints.

## Ready

This plugin report readiness to the *ready* plugin. This will happen after a gRPC stream has been
established to the control plane.

## Examples

~~~
lb.example.org {
    traffic grpc://127.0.0.1:18000 {
        id test-id
    }
    debug
    log
}
~~~

This will load balance any names under `lb.example.org` using the data from the manager running on
localhost on port 18000. The node ID will be `test-id` and no TLS will be used. Assuming a
management server returns config for `web` cluster, you can query CoreDNS for it, below we do an
address lookup, which returns an address for the endpoint. The second example shows a SRV lookup
which returns all endpoints.

~~~ sh
$ dig web.lb.example.org +noall +answer
web.lb.example.org.	5	IN	A	127.0.1.1

$ dig web.lb.example.org SRV +noall +answer +additional
web.lb.example.org.	5	IN	SRV	100 100 18008 endpoint-0.web.lb.example.org.
web.lb.example.org.	5	IN	SRV	100 100 18008 endpoint-1.web.lb.example.org.
web.lb.example.org.	5	IN	SRV	100 100 18008 endpoint-2.web.lb.example.org.

endpoint-0.web.lb.example.org. 5 IN	A	127.0.1.1
endpoint-1.web.lb.example.org. 5 IN	A	127.0.1.2
endpoint-2.web.lb.example.org. 5 IN	A	127.0.2.1
~~~

## Bugs

Credentials are not implemented. Bootstrapping is not fully implemented, *traffic* will connect to
the first working **TO** address, but then stops short of re-connecting to the endpoints of the
management **CLUSTER**.

Load reporting is not supported for the following reason: A DNS query is done by a resolver.
Behind this resolver (which can also cache) there may be many clients that will use this reply. The
responding server (CoreDNS) has no idea how many clients use this resolver. So reporting a load of
+1 on the CoreDNS side can results in anything from 1 to 1000+ of queries on the endpoint, making
the load reporting from the *traffic* plugin highly inaccurate. Hence it is not done.

## Also See

A Envoy management server and command line interface can be found on
[GitHub](https://github.com/miekg/xds).
