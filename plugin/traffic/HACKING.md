Hacking on *traffic*

Repos used:

<https://github.com/envoyproxy/go-control-plane>
:   implements control plane, has testing stuff in pkg/test/main (iirc).

<https://github.com/grpc/grpc-go/tree/master/xds/internal/client>
:   implements client for xDS - can probably list all code out from there.

To see if things are working start the testing control plane from go-control-plane:

https://github.com/envoyproxy/envoy/blob/master/api/API_OVERVIEW.md

https://github.com/envoyproxy/learnenvoy/blob/master/_articles/service-discovery.md


Cluster: A cluster is a group of logically similar endpoints that Envoy connects to. In v2, RDS
routes points to clusters, CDS provides cluster configuration and Envoy discovers the cluster
members via EDS.

# Testing

~~~ sh
$ cd ~/src/github.com/envoyproxy/go-control-plane
% make integration.xds
~~~

This runs a binary from pkg/test/main. Now we're testing xDS, but there is also aDS (which does
everything including xDS). I'm still figuring out what do to here.

The script stops, unless you have Envoy installed (which I haven't), but you can run it manually:

~~~ sh
./bin/test --xds=xds --runtimes=1 -debug  # for xds
~~~

This fails with `timeout waiting for the first request`, means you're consumer wasn't quick enough
in asking for xDS assignments.
