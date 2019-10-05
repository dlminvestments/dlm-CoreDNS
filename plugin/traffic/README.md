# traffic

## Name

*traffic* - handout addresses according to assignments.

## Description

The *traffic* plugin is a load balancer that allows traffic steering, weighted responses and
draining of endpoints. Endpoints are IP:port pairs. *Traffic* works as an overlay on top of other
plugins, it does not mandate any storage by itself.

*Traffic* receives (via gRPC?) *assignments* that define the weight of the endpoints in services.
The plugin takes care of handing out responses that adhere to these assignments. Assignments will
need to be updated frequently, without new updates *traffic* will hand out responses according to
the last received assignment. When there are no assignments for a service name (yet), the responses
will also be modified (see below).

An assignment covers a "service name", which is a domain name. For each service a number of backends
are expected. A backend is defined as an IP:port pair Each backend comes with a integer indicating
it relative weight. A zero means the backend exists, but should not be handed out (drain it).

*Traffic* will load balance A and AAAA queries. known to the plugin. It will return precisely one
record in a response, which is the optimal record according to the assignments and previously handed
out responses. If a service should be load balanced, but no assignment can be found a random record
from the *answer section* will be choosen.

Every message that is handled by the *traffic* plugin will have all it's TTLs set to 5 seconds,
any authority section is removed and all RRSIGs are removed from it.

The *traffic* plugin has no notion of draining, drop overload and anything that advanced, *it just
acts upon assignments*. This is means that if a backend goes down and *traffic* has not seen a new
assignment yet, it will still include this backend in responses.

## Syntax

~~~
traffic
~~~

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

Assignments are given in protobuf format, but here is an example in YAML conveying the same
information. This is an example assignment for the service "www.example.org".

~~~ yaml
assignments:
    - service: www.example.org
        - backend: 192.168.1.1:443
            assign: 4
          backend: 192.168.1.2:443
            assign: 6
          backend: 192.168.1.3:443
            assign: 0
~~~

This particular one has 3 backends, one of which is to be drained (192.168.1.3). the two remaining
ones have a non zero weighted assignment. We use "Weighted Random Selection" to select a backend:

* Add up all the weights for all the items in the list (here 8).
* Pick a number at random between 1 and the sum of the weights.
* Iterate over the items
* For the current item, subtract the item's weight from the random number.
* If less or zero pick this item, other continue with the next item.

On seeing a query for a service, *traffic* will track the reply. When it returns with an answer
*traffic* will rewrite it (and discard of any RRSIGs). Using the assignments the answer section will
be rewritten as such:

* A backend will be picked using the algorithm from above.
* The TTL on the response will be 5s for all included records.
* According to previous responses for this service and the relative weights of each backends the
  best backend will be put in the response.
* If after the selection *no* backends are available an NODATA response will be sent. An SOA
  record will be synthesised, and a low TTL (and negative TTL) of 5 seconds will be set.

TTL rewriting always? TODO.
Authority section will be removed.
If no assignment, randomly pick an address
other types then A and AAAA, like SRV - do the same selection.

## Bugs

This plugin does not play nice with DNSSEC - if the backend returns signatures with the answer; they
will be stripped. You can optionally sign responses on the fly by using the *dnssec* plugin.

## Also See

This is a [post on weighted random
selection](https://medium.com/@peterkellyonline/weighted-random-selection-3ff222917eb6).

## TODO

Should we add source address information (geographical load balancing) to the assignment? This can
be handled be having each backend specify an optional source range there this record should be used.
For IPv4 this must a /24 for IPv6 a /64.

Other points that require more attention:

* deleting assignments?
* last known good assignment (esp with deleting assignments)?
