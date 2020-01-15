This code is copied from
[https://github.com/grpc/grpc-go/tree/master/xds](https://github.com/grpc/grpc-go/tree/master/xds).
Grpc-go is also a consumer of the Envoy xDS data and acts upon it.

The *traffic* plugin only cares about clusters and endpoints, the following bits are deleted:

* lDS; listener discovery is not used here.
* rDS: routes have no use for DNS responses.

Load reporting is also not implemented, although this can be done on the DNS level.
