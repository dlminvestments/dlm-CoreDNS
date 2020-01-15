Hacking on *traffic*

Repos used:

<https://github.com/envoyproxy/go-control-plane>
:   implements control plane, has testing stuff in pkg/test/main (iirc).

<https://github.com/grpc/grpc-go/tree/master/xds/internal/client>
:   implements client for xDS - much of this code has been reused here.

I found these website useful while working on this.

* https://github.com/envoyproxy/envoy/blob/master/api/API_OVERVIEW.md
* https://github.com/envoyproxy/learnenvoy/blob/master/_articles/service-discovery.md
* This was *really* helpful: https://www.envoyproxy.io/docs/envoy/v1.11.2/api-docs/xds_protocol

# Testing

Assuming you have envoyproxy/go-control-plane checked out somewhere, then:

~~~ sh
% cd ~/src/github.com/envoyproxy/go-control-plane/pkg/test/main
% go build
% ./main --xds=ads --runtimes=2 -debug
~~~

This runs a binary from pkg/test/main. Now we're testing aDS. Everything is using gRPC with TLS
disabled: `grpc.WithInsecure()`. The test binary runs on port 18000 on localhost; all these things
are currently hardcoded in the *traffic* plugin. This will be factored out into config as some
point. Another thing that is hardcoded is the use of the "example.org" domain.

Then for CoreDNS, check out the `traffic` branch, create a Corefile:

~~~ Corefile
example.org {
    traffic
    debug
}
~~~

Start CoreDNS (`coredns -conf Corefile -dns.port=1053`), and see logging/debugging flow by; the
test binary should also spew out a bunch of things. CoreDNS willl build up a list of cluster and
endpoints. Next you can query it:

~~~ sh
% dig @localhost -p 1053 cluster-v0-0.example.org A
;; QUESTION SECTION:
;cluster-v0-0.example.org.	IN	A

;; ANSWER SECTION:
cluster-v0-0.example.org. 5	IN	A	127.0.0.1
~~~

Note: the xds/test binary is a go-control-plane binary with added debugging that I'm using for
testing.
