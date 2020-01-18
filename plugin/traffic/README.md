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

Each DNS response contains a single IP address that's considered the best one. *Traffic* will load
balance A and AAAA queries. The TTL on these answer is set to 5s. It will only return successful
responses either with an answer or otherwise a NODATA response. Queries for non-existent clusters
get a NXDOMAIN, where the minimal TTL is also set to 5s.

The *traffic* plugin has no notion of draining, drop overload and anything that advanced, *it just
acts upon assignments*. This is means that if a endpoint goes down and *traffic* has not seen a new
assignment yet, it will still include this endpoint address in responses.

Load reporting is not supported for the following reason. A DNS query is done by a resolver.
Behind this resolver (which can also cache) there may be many clients that will use this reply. The
responding server (CoreDNS) has no idea how many clients use this resolver. So reporting a load of
+1 on the CoreDNS side can results in anything from 1 to 1000+ of queries on the endpoint, making
the load reporting from *trafifc* highly inaccurate.

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
    tls CERT KEY CA
    tls_servername NAME
}
~~~

*  node **ID** is how *traffic* identifies itself to the control plane. This defaults to `coredns`.
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

## Naming Clusters

When a cluster is named this usually consists out of a single word, i.e. "cluster-v0", or "web".
The *traffic* plugins uses the name(s) specified in the Server Block to create fully qualified
domain names. For example if the Server Block specifies `lb.example.org` as one of the names,
and "cluster-v0" is one of the load balanced cluster, *traffic* will respond to query asking for
`cluster-v0.lb.example.org.` and the same goes for `web`; `web.lb.example.org`.

## Metrics

What metrics should we do? If any? Number of clusters? Number of endpoints and health?

## Ready

This plugin report readiness to the ready plugin. This will happen after a gRPC stream has been
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
is not implemented.

## TODO

* metrics?
* credentials (other than TLS) - how/what?
* is the protocol correctly implemented? Should we not have a 10s tick, but wait for responses from
  the control plane?
