# traffic

## Name

*traffic* - handout addresses according to assignments from Envoy's xDS.

## Description

The *traffic* plugin is a balancer that allows traffic steering, weighted responses
and draining of clusters. The cluster information is retrieved from a service
discovery manager that implements the service discovery protocols that Envoy
[implements](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol).

A Cluster is defined as: "A cluster is a group of logically similar endpoints that Envoy connects
to. Each cluster has a name, which *traffic* extends to be a domain name.

The use case for this plugin is when a cluster has endpoints running in multiple
(Kubernetes?) clusters and you need to steer traffic to (or away) from these endpoints, i.e.
endpoint A needs to be upgraded, so all traffic to it is drained. Or the entire Kubernetes needs to
upgraded, and *all* endpoints need to be drained from it.

*Traffic* discovers the endpoints via Envoy's xDS protocol. Endpoints and clusters are discovered
every 10 seconds. The plugin hands out responses that adhere to these assignments. Each DNS response
contains a single IP address that's considered the best one. *Traffic* will load balance A and AAAA
queries. The TTL on these answer is set to 5s.

The *traffic* plugin has no notion of draining, drop overload and anything that advanced, *it just
acts upon assignments*. This is means that if a endpoint goes down and *traffic* has not seen a new
assignment yet, it will still include this endpoint address in responses.

## Syntax

~~~
traffic
~~~

The extended syntax (not implemented; everything is hard-coded at the moment):

~~~
traffic {
    server grpc://dsdsd <creds>
    id ID
 }
~~~

* id **ID** is how *traffic* identifies itself to the control plane.


## Examples

~~~ corefile
example.org {
    traffic
    debug
    log
}
~~~

This will add load balancing for domains under example.org; the upstream information comes from
10.12.13.14; depending on received assignments, replies will be let through as-is or are load balanced.

## Bugs

Priority from ClusterLoadAssignments is not used. Locality is also not used. Health status of the
endpoints is ignore (for now).

Load reporting via xDS is not supported; this can be implemented, but there are some things that make
this difficult. A single (DNS) query is done by a resolver. Behind this resolver there may be many
clients that will use this reply, the responding server (CoreDNS) has no idea how many clients use
this resolver. So reporting a load of +1 on the CoreDNS side can be anything from 1 to 1000+, making
the load reporting highly inaccurate.

## Also See

The following documents provide some background on Envoy's control plane.

* https://github.com/envoyproxy/go-control-plane
* https://blog.christianposta.com/envoy/guidance-for-building-a-control-plane-to-manage-envoy-proxy-based-infrastructure/
* https://github.com/envoyproxy/envoy/blob/442f9fcf21a5f091cec3fe9913ff309e02288659/api/envoy/api/v2/discovery.proto#L63
