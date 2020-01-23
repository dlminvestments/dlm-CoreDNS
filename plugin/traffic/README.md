# traffic

## Name

*traffic* - handout addresses according to assignments from Envoy's xDS.

## Description

The *traffic* plugin is a balancer that allows traffic steering, weighted responses
and draining of clusters. The cluster information is retrieved from a service
discovery manager that implements the service discovery protocols from Envoy
[implements](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol). It connect to the
manager using the Aggregated Discovery Service (ADS) protocol.

A Cluster in Envoy is defined as: "A group of logically similar endpoints that Envoy connects to."
Each cluster has a name, which *traffic* extends to be a domain name. See "Naming Clusters" below.

The use case for this plugin is when a cluster has endpoints running in multiple (Kubernetes?)
clusters and you need to steer traffic to (or away) from these endpoints, i.e. endpoint A needs to
be upgraded, so all traffic to it is drained. Or the entire Kubernetes needs to upgraded, and *all*
endpoints need to be drained from it.

*Traffic* discovers the endpoints via Envoy's xDS protocol (using ADS). Endpoints and clusters are
discovered every 10 seconds. The plugin hands out responses that adhere to these assignments. Only
endpoints that are *healthy* are handed out.

For A and AAAA queries each DNS response contains a single IP address that's considered the best
one. The TTL on these answer is set to 5s. It will only return successful responses either with an
answer or otherwise a NODATA response. Queries for non-existent clusters get a NXDOMAIN, where the
minimal TTL is also set to 5s.

For SRV queries all healthy backends will be returned - assuming the client doing the query is smart
enough to select the best one. When SRV records are returned, the endpoint DNS names are synthesized
`endpoint-<N>.<cluster>.<zone>` that carries the IP address. Querying for these synthesized names
works as well.

Load reporting is not supported for the following reason. A DNS query is done by a resolver.
Behind this resolver (which can also cache) there may be many clients that will use this reply. The
responding server (CoreDNS) has no idea how many clients use this resolver. So reporting a load of
+1 on the CoreDNS side can results in anything from 1 to 1000+ of queries on the endpoint, making
the load reporting from *traffic* highly inaccurate.

## Syntax

~~~
traffic TO...
~~~

This enabled the *traffic* plugin, with a default node ID of `coredns` and no TLS.

*  **TO...** are the control plane endpoints to connect to. These must start with `grpc://`. The
  port number defaults to 443, if not specified.

The extended syntax is available if you want more control.

~~~
traffic TO... {
    node ID
    locality REGION,ZONE,SUBZONE [REGION,ZONE,SUBZONE]...
    tls CERT KEY CA
    tls_servername NAME
    ignore_health
}
~~~

*  `node` **ID** is how *traffic* identifies itself to the control plane. This defaults to `coredns`.
*  `locality` has a list of **REGION,ZONE,SUBZONE**s. These tell *traffic* where its running and what should be
   considered local traffic. Each **REGION,ZONE,SUBZONE** will be used to match clusters again while generating
   responses. The list should descend in proximity. A `*` describes a wild card match. I.e. when
   there are 3 regions, US, EU, ASIA, and this CoreDNS is running in EU, you can use:
   `locality EU,*,* US,*,*, ASIA,*,*`. Only when the cluster's locality isn't UNKNOWN will this
   matching happen.
* `tls` **CERT** **KEY** **CA** define the TLS properties for gRPC connection. If this is omitted an
  insecure connection is attempted. From 0 to 3 arguments can be provided with the meaning as described below

  * `tls` - no client authentication is used, and the system CAs are used to verify the server certificate
  * `tls` **CA** - no client authentication is used, and the file CA is used to verify the server certificate
  * `tls` **CERT** **KEY** - client authentication is used with the specified cert/key pair.
    The server certificate is verified with the system CAs.
  * `tls` **CERT** **KEY** **CA** - client authentication is used with the specified cert/key pair.
    The server certificate is verified using the specified CA file.

* `tls_servername` **NAME** allows you to set a server name in the TLS configuration. This is needed
  because *traffic* connects to an IP address, so it can't infer the server name from it.
* `ignore_health` can be enabled to ignore endpoint health status, this can aid in debugging.

## Naming Clusters

When a cluster is named this usually consists out of a single word, i.e. "cluster-v0", or "web".
The *traffic* plugins uses the name(s) specified in the Server Block to create fully qualified
domain names. For example if the Server Block specifies `lb.example.org` as one of the names,
and "cluster-v0" is one of the load balanced cluster, *traffic* will respond to query asking for
`cluster-v0.lb.example.org.` and the same goes for `web`; `web.lb.example.org`.

## Localized Endpoints

Endpoints can be grouped by location, this location information is used if the `locality` property
is used in the configuration.

## Matching Algorithm

How are clients match against the data we receive from xDS endpoint? Ignoring `locality` for now,
it will go through the following steps:

1. Does the cluster exist? If not return NXDOMAIN, otherwise continue.
2. Run through the endpoints, discard any endpoints that are not HEALTHY. If we are left with no
   endpoint return a NODATA response, otherwise continue.
3. If weights are assigned use those to pick an endpoint, otherwise randomly pick one and return a
   response to the client.

If `locality` *has* been specified there is an extra step between 2 and 3.

2a. Match the endpoints using the locality that groups several of them, it's the most specific match
    from left to right in the `locality` list; if no **REGION,ZONE,SUBZONE** matches then try
    **REGION,ZONE** and then **REGION**. If still not match, move on the to next one. If we found
    none, we continue with step 4 above, ignoring any locality.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric are exported:

* `coredns_traffic_clusters_tracked{}` the number of tracked clusters.

## Ready

This plugin report readiness to the *ready* plugin. This will happen after a gRPC stream has been
established to the control plane.

## Examples

~~~
lb.example.org {
    traffic grpc://127.0.0.1:18000 {
        node test-id
    }
    debug
    log
}
~~~

This will load balance any names under `lb.example.org` using the data from the manager running on
localhost on port 18000. The node ID will be `test-id` and no TLS will be used.

## Also See

The following documents provide some background on Envoy's control plane.

 *  <https://github.com/envoyproxy/go-control-plane>

 *  <https://blog.christianposta.com/envoy/guidance-for-building-a-control-plane-to-manage-envoy-proxy-based-infrastructure/>

 *  <https://github.com/envoyproxy/envoy/blob/442f9fcf21a5f091cec3fe9913ff309e02288659/api/envoy/api/v2/discovery.proto#L63>

## Bugs

Priority and locality information from ClusterLoadAssignments is not used. Multiple **TO** addresses
is not implemented. Credentials are not implemented.
