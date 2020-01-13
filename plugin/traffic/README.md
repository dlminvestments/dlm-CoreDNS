# traffic

## Name

*traffic* - handout addresses according to assignments.

## Description

The *traffic* plugin is a load balancer that allows traffic steering, weighted responses and
draining of endpoints. The use case for this plugin is when a service is running in multiple
(Kubernetes?) clusters and need to steer traffic to (or away) from these, i.e. cluster A needs to be
upgraded, so all traffic to it is drained, while cluster B now takes on all the extra load. After
the maintenance cluster A is simply undrained.

*Traffic* discovers the endpoints via the Envoy xDS protocol, specifically messages of the type
"envoy.api.v2.ClusterLoadAssignment", these contain endpoints and an (optional) weight for each.
The `cluster_name` or `service_name` for a service must be a domain name. (TODO: check is this is
already the case). The plugin hands out responses that adhere to these assignments.
Assignments will need to be updated frequently, as discussed the [Envoy xDS
protocol](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol) documentation.

Multiple endpoints for a service may exist; for every query *traffic* will hand out exactly one
address. When there are no assignments for a service name (yet), the responses will also be modified
(see below).

*Traffic* will load balance A and AAAA queries. As said, it will return precisely one record in a
response. If a service should be load balanced, but no assignment can be found a random record from
the *answer section* will be choosen.

Every message that is handled by the *traffic* plugin will have its TTLs set to 5 seconds, the
authority section, and all RRSIGs are removed from it.

The *traffic* plugin has no notion of draining, drop overload and anything that advanced, *it just
acts upon assignments*. This is means that if a endpoint goes down and *traffic* has not seen a new
assignment yet, it will still include this endpoint address in responses.

Findign the xDS endpoint.

## Syntax

~~~
traffic
~~~

The extended syntax:

~~~
traffic {
    server grpc://dsdsd <creds>
    id ID
 }
~~~

* id **ID** is how *traffic* identifies itself to the control plane.

This enables traffic load balancing for all (sub-)domains named in the server block.

## Examples

~~~ corefile
example.org {
    traffic
    forward . 10.12.13.14
}
~~~

This will add load balancing for domains under example.org; the upstream information comes from
10.12.13.14; depending on received assignments, replies will be let through as-is or are load balanced.

## Assignments

Assignments are streamed for a service that implements the xDS protocol, *traffic* will bla bla.
TODO.

Picking an endpoint is done as follows: (still true for xDs - check afer implementing things)

* include spiffy algorithm.

On seeing a query for a service, *traffic* will track the reply. When it returns with an answer
*traffic* will rewrite it (and discard of any RRSIGs). Using the assignments the answer section will
be rewritten as such:

* A endpoint will be picked using the algorithm from above.
* The TTL on the response will be 5s for all included records.
* According to previous responses for this service and the relative weights of each endpoints the
  best endpoint will be put in the response.
* If after the selection *no* endpoints are available an NODATA response will be sent. An SOA
  record will be synthesised, and a low TTL (and negative TTL) of 5 seconds will be set.

Authority section will be removed.
If no assignment, randomly pick an address
other types then A and AAAA, like SRV - do the same selection.

## Limitations

Loadreporting via xDS is not supported; this can be implemented, but there are some things that make
this difficult. A single (DNS) query is done by a resolver. Behind this resolver there may be many
clients that will use this assignment.


## Bugs

This plugin does not play nice with DNSSEC - if the endpoint returns signatures with the answer; they
will be stripped. You can optionally sign responses on the fly by using the *dnssec* plugin.

## Also See

* https://github.com/envoyproxy/go-control-plane
* https://blog.christianposta.com/envoy/guidance-for-building-a-control-plane-to-manage-envoy-proxy-based-infrastructure/
* https://github.com/envoyproxy/envoy/blob/442f9fcf21a5f091cec3fe9913ff309e02288659/api/envoy/api/v2/discovery.proto#L63
* This is a [post on weighted random selection](https://medium.com/@peterkellyonline/weighted-random-selection-3ff222917eb6).

# TODO

* wording: cluster, endpoints, assignments, service_name are all used and roughly mean the same
 thing; unify this.
