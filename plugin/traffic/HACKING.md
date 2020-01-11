Hacking on *traffic*

Repos used:

<https://github.com/envoyproxy/go-control-plane>
:   implements control plane, has testing stuff in pkg/test/main (iirc).

<https://github.com/grpc/grpc-go/tree/master/xds/internal/client>
:   implements client for xDS - can probably list all code out from there.

To see if things are working start the testing control plane from go-control-plane:
