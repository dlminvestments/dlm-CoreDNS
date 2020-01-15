Hacking on *traffic*

Repos used:

<https://github.com/envoyproxy/go-control-plane>
:   implements control plane, has testing stuff in pkg/test/main (iirc).

<https://github.com/grpc/grpc-go/tree/master/xds/internal/client>
:   implements client for xDS - much of this code has been reused here.

To see if things are working start the testing control plane from go-control-plane:

* https://github.com/envoyproxy/envoy/blob/master/api/API_OVERVIEW.md
* https://github.com/envoyproxy/learnenvoy/blob/master/_articles/service-discovery.md
* This was really helpful: https://www.envoyproxy.io/docs/envoy/v1.11.2/api-docs/xds_protocol

Cluster: A cluster is a group of logically similar endpoints that Envoy connects to. In v2, RDS
routes points to clusters, CDS provides cluster configuration and Envoy discovers the cluster
members via EDS.

# Testing

~~~ sh
% cd ~/src/github.com/envoyproxy/go-control-plane/pkg/test/main
% go build
% ./main --xds=ads --runtimes=2 -debug
~~~

This runs a binary from pkg/test/main. Now we're testing aDS. Everything is using gRPC with TLS,
`grpc.WithInsecure()`. The binary runs on port 18000 on localhost; all these things are currently
hardcoded in the *traffic* plugin. This will be factored out into config as some point.

Then for CoreDNS, check out the `traffic` branch, create a Corefile:

~~~ Corefile
example.org {
    traffic
    debug
}
~~~

Start CoreDNS, and see logging/debugging flow by; the test binary should also spew out a bunch of
things. CoreDNS willl build up a list of cluster and endpoints. Next you can query it.

TODO
