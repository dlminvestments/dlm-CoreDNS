# traffic

## Name

*traffic* - handout addresses according to assignments from Envoy's xDS.

## Description

The *traffic* plugin is a balancer that allows traffic steering, weighted responses and draining
of clusters. A cluster in Envoy is defined as: "A group of logically similar endpoints that Envoy
connects to." Each cluster has a name, which *traffic* extends to be a domain name. See "Naming
Clusters" below.

The use case for this plugin is when a cluster has endpoints running in multiple (Kubernetes?)
clusters and you need to steer traffic to (or away) from these endpoints, i.e. endpoint A needs to
be upgraded, so all traffic to it is drained. Or the entire Kubernetes needs to upgraded, and *all*
endpoints need to be drained from it.

The cluster information is retrieved from a service discovery manager that implements the service
discovery [protocols from Envoy implements](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol).
It connects to the manager using the Aggregated Discovery Service (ADS) protocol. Endpoints and
clusters are discovered every 10 seconds. The plugin hands out responses that adhere to these
assignments. Only endpoints that are *healthy* are handed out.

Note that the manager *itself* is also a cluster that is managed *by the management server*. This is
the *management cluster* (see `cluster` below in "Syntax"). By default the name for cluster is `xds`.
When bootstrapping *traffic* tries to retrieve the cluster endpoints for the management cluster.
This continues in the background and *traffic* is smart enough to reconnect on failures or updates
cluster configuration. If the `xds` management cluster can't be found on start up, *traffic* returns a
fatal error.

For A and AAAA queries each DNS response contains a single IP address that's considered the best
one. The TTL on these answer is set to 5s. It will only return successful responses either with an
answer or otherwise a NODATA response. Queries for non-existent clusters get a NXDOMAIN, where the
minimal TTL is also set to 5s.

For SRV queries all healthy backends will be returned - assuming the client doing the query is smart
enough to select the best one. When SRV records are returned, the endpoint DNS names are synthesized
`endpoint-<N>.<cluster>.<zone>` that carries the IP address. Querying for these synthesized names
works as well.

[gRPC LB SRV records](https://github.com/grpc/proposal/blob/master/A5-grpclb-in-dns.md) are
supported and returned by the *traffic* plugin for all clusters. The returned endpoints are,
however, the ones from the management cluster as these must implement gRPC LB.

*Traffic* implements version 3 of the xDS API. It works with the management server as written in
<https://github.com/miekg/xds>.

If *traffic*'s `locality` has been set the answers can be localized.

## Syntax

~~~
traffic TO...
~~~

This enabled the *traffic* plugin, with a default node ID of `coredns` and no TLS.

 *  **TO...** are the control plane endpoints to bootstrap from. These must start with `grpc://`. The
    port number defaults to 443, if not specified. These endpoint will be tried in the order given.
    First successful connection will be used to resolve the management cluster `xds`.

The extended syntax is available if you want more control.

~~~
traffic TO... {
    cluster CLUSTER
    id ID
    locality REGION[,ZONE[,SUBZONE]] [REGION[,ZONE[,SUBZONE]]]...
    tls CERT KEY CA
    tls_servername NAME
    ignore_health
}
~~~

 *  `cluster` **CLUSTER** define the name of the management cluster. By default this is `xds`.

 *  `id` **ID** is how *traffic* identifies itself to the control plane. This defaults to
    `coredns`.

 *  `locality` has a list of **REGION,ZONE,SUBZONE** sets. These tell *traffic* where its running
    and what should be considered local traffic. Each **REGION,ZONE,SUBZONE** set will be used
    to match clusters against while generating responses. The list should descend in proximity.
    **ZONE** or **ZONE** *and* **SUBZONE** may be omitted. This signifies a wild card match.
    I.e. when there are 3 regions, US, EU, ASIA, and this CoreDNS is running in EU, you can use:
    `locality EU US ASIA`. Each list must be separated using spaces. The elements within a set
    should be separated with only a comma.

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

 *  `ignore_health` can be enabled to ignore endpoint health status, this can aid in debugging.

## Naming Clusters

When a cluster is named this usually consists out of a single word, i.e. "cluster-v0", or "web".
The *traffic* plugins uses the name(s) specified in the Server Block to create fully qualified
domain names. For example if the Server Block specifies `lb.example.org` as one of the names,
and "cluster-v0" is one of the load balanced cluster, *traffic* will respond to queries asking for
`cluster-v0.lb.example.org.` and the same goes for `web`; `web.lb.example.org`.

For SRV queries all endpoints are returned, the SRV target names are synthesized:
`endpoint-<N>.web.lb.example.org` to take the example from above. *N* is an integer starting with 0.

For the management cluster `_grpclb._tcp.<cluster>.<name>` will also be resolved in the same way as
normal SRV queries. This special case is done because gRPC lib

the gRPC LBs are. For each **TO** in the configuration *traffic* will return a SRV record. The
target name in the SRV are synthesized as well, using `grpclb-N` to prefix the zone from the Corefile,
i.e. `grpclb-0.lb.example.org` will be the gRPC name when using `lb.example.org` in the configuration.
Each `grpclb-N` target will have one address record, namely the one specified in the configuration.

## Localized Endpoints

Endpoints can be grouped by location, this location information is used if the `locality` property
is used in the configuration.

## Matching Algorithm

How are clients match against the data we receive from xDS endpoint? Ignoring `locality` for now, it
will go through the following steps:

1.  Does the cluster exist? If not return NXDOMAIN, otherwise continue.

2.  Run through the endpoints, discard any endpoints that are not HEALTHY. If we are left with no
    endpoint return a NODATA response, otherwise continue.

3.  If weights are assigned use those to pick an endpoint, otherwise randomly pick one and return a
    response to the client.

If `locality` *has* been specified there is an extra step between 2 and 3.

2a. Match the endpoints using the locality that groups several of them, it's the most specific
match from left to right in the `locality` list; if no **REGION,ZONE,SUBZONE** matches then try
**REGION,ZONE** and then **REGION**. If still not match, move on the to next one. If we found none,
we continue with step 4 above, ignoring any locality.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

 *  `coredns_traffic_clusters_tracked{}` the number of tracked clusters.
 *  `coredns_traffic_endpoints_tracked{}` the number of tracked clusters.

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
which returns all endpoints. The third shows what gRPC will ask for when looking for load balancers.

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

$ dig _grpclb._tcp.web.lb.example.org SRV +noall +answer +additional

_grpclb._tcp.web.lb.example.org. 5 IN	SRV	100 100 18008 endpoint-0.xds.lb.example.org.
_grpclb._tcp.web.lb.example.org. 5 IN	SRV	100 100 18008 endpoint-1.xds.lb.example.org.
_grpclb._tcp.web.lb.example.org. 5 IN	SRV	100 100 18008 endpoint-2.xds.lb.example.org.

endpoint-0.xds.lb.example.org. 5 IN	A	10.0.1.1
endpoint-1.xds.lb.example.org. 5 IN	A	10.0.1.2
endpoint-2.xds.lb.example.org. 5 IN	A	10.0.2.1
~~~

## Bugs

Priority and locality information from ClusterLoadAssignments is not used. Credentials are not
implemented.

Load reporting is not supported for the following reason: A DNS query is done by a resolver.
Behind this resolver (which can also cache) there may be many clients that will use this reply. The
responding server (CoreDNS) has no idea how many clients use this resolver. So reporting a load of
+1 on the CoreDNS side can results in anything from 1 to 1000+ of queries on the endpoint, making
the load reporting from *traffic* highly inaccurate.

Bootstrapping is not fully implemented, *traffic* will connect to the first working **TO** addresss,
but then stops short of re-connecting to he endpoints is received for the management **CLUSTER**.

## Also See

A Envoy management server and command line interface can be found on
[GitHub](https://github.com/miekg/xds).
