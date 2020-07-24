/*
This package contains code copied from github.com/grpc/grpc-co. The license for that code is:

Copyright 2019 gRPC authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package xds implements a bidirectional stream to an envoy ADS management endpoint. It will stream
// updates (CDS and EDS) from there to help load balance responses to DNS clients.
package xds

import (
	"context"
	"sync"

	"github.com/coredns/coredns/coremain"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	xdspb2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	corepb2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	adspb2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc"
)

var log = clog.NewWithPlugin("traffic: xds")

const (
	clusterType  = "type.googleapis.com/envoy.api.v2.Cluster"
	endpointType = "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"
)

type adsStream adspb2.AggregatedDiscoveryService_StreamAggregatedResourcesClient

// Client talks to the grpc manager's endpoint to get load assignments.
type Client struct {
	cc          *grpc.ClientConn
	ctx         context.Context
	node        *corepb2.Node
	cancel      context.CancelFunc
	stop        chan struct{}
	to          string       // upstream hosts, mostly here for logging purposes
	mu          sync.RWMutex // protects everything below
	assignments *assignment  // assignments contains the current clusters and endpoints
	version     map[string]string
	nonce       map[string]string
	synced      bool // true when we first successfully got a stream
}

// New returns a new client that's dialed to addr using node as the local identifier.
func New(addr, node string, opts ...grpc.DialOption) (*Client, error) {
	cc, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, err
	}
	c := &Client{cc: cc, to: addr, node: &corepb2.Node{Id: node, UserAgentName: "CoreDNS", UserAgentVersionType: &corepb2.Node_UserAgentVersion{UserAgentVersion: coremain.CoreVersion}}}
	c.assignments = &assignment{cla: make(map[string]*xdspb2.ClusterLoadAssignment)}
	c.version, c.nonce = make(map[string]string), make(map[string]string)
	c.ctx, c.cancel = context.WithCancel(context.Background())

	return c, nil
}

// Stop stops all goroutines and closes the connection to the upstream manager.
func (c *Client) Stop() error { c.cancel(); return c.cc.Close() }

// Run starts all goroutines and gathers the clusters and endpoint information from the upstream manager.
func (c *Client) Run() error {
	first := true
	for {
		select {
		case <-c.ctx.Done():
			return nil
		default:
		}

		cli := adspb2.NewAggregatedDiscoveryServiceClient(c.cc)
		stream, err := cli.StreamAggregatedResources(c.ctx)
		if err != nil {
			return err
		}

		if first {
			// send first request, to create stream, then wait for ADS to send us updates.
			if err := c.clusterDiscovery(stream, c.Version(clusterType), c.Nonce(clusterType), []string{}); err != nil {
				return err
			}
			log.Infof("gRPC stream established to %q", c.to) // might fail??
			c.setSynced()
			first = false
		}

		if err := c.receive(stream); err != nil {
			return err
		}
	}
}

// clusterDiscovery sends a cluster DiscoveryRequest on the stream.
func (c *Client) clusterDiscovery(stream adsStream, version, nonce string, clusters []string) error {
	req := &xdspb2.DiscoveryRequest{
		Node:          c.node,
		TypeUrl:       clusterType,
		ResourceNames: clusters, // empty for all
		VersionInfo:   version,
		ResponseNonce: nonce,
	}
	return stream.Send(req)
}

// endpointDiscovery sends a endpoint DiscoveryRequest on the stream.
func (c *Client) endpointDiscovery(stream adsStream, version, nonce string, clusters []string) error {
	req := &xdspb2.DiscoveryRequest{
		Node:          c.node,
		TypeUrl:       endpointType,
		ResourceNames: clusters,
		VersionInfo:   version,
		ResponseNonce: nonce,
	}
	return stream.Send(req)
}

// receive receives from the stream, it handles both cluster and endpoint DiscoveryResponses.
func (c *Client) receive(stream adsStream) error {
	for {
		resp, err := stream.Recv()
		if err != nil {
			return err
		}

		switch resp.GetTypeUrl() {
		case clusterType:
			a := NewAssignment()
			for _, r := range resp.GetResources() {
				var any ptypes.DynamicAny
				if err := ptypes.UnmarshalAny(r, &any); err != nil {
					log.Debugf("Failed to unmarshal cluster discovery: %s", err)
					continue
				}
				cluster, ok := any.Message.(*xdspb2.Cluster)
				if !ok {
					continue
				}
				a.SetClusterLoadAssignment(cluster.GetName(), nil)
			}
			// set our local administration and ack the reply. Empty version would signal NACK.
			c.SetNonce(clusterType, resp.GetNonce())
			c.SetVersion(clusterType, resp.GetVersionInfo())
			c.SetAssignments(a)
			c.clusterDiscovery(stream, resp.GetVersionInfo(), resp.GetNonce(), a.clusters())

			log.Debugf("Cluster discovery processed with %d resources, version %q and nonce %q", len(resp.GetResources()), c.Version(clusterType), c.Nonce(clusterType))
			ClusterGauge.Set(float64(len(resp.GetResources())))

			// now kick off discovery for endpoints
			if err := c.endpointDiscovery(stream, c.Version(endpointType), c.Nonce(endpointType), a.clusters()); err != nil {
				log.Debug(err)
			}
		case endpointType:
			for _, r := range resp.GetResources() {
				var any ptypes.DynamicAny
				if err := ptypes.UnmarshalAny(r, &any); err != nil {
					log.Debugf("Failed to unmarshal endpoint discovery: %s", err)
					continue
				}
				cla, ok := any.Message.(*xdspb2.ClusterLoadAssignment)
				if !ok {
					// TODO warn/err here?
					continue
				}
				c.assignments.SetClusterLoadAssignment(cla.GetClusterName(), cla)

			}
			// set our local administration and ack the reply. Empty version would signal NACK.
			c.SetNonce(endpointType, resp.GetNonce())
			c.SetVersion(endpointType, resp.GetVersionInfo())

			log.Debugf("Endpoint discovery processed with %d resources, version %q and nonce %q", len(resp.GetResources()), c.Version(endpointType), c.Nonce(endpointType))
			EndpointGauge.Set(float64(len(resp.GetResources())))

		default:
			// ignore anything we don't know how to process. Probably should NACK these properly.
		}
	}
}

// Select returns an address that is deemed to be the correct one for this cluster. The returned
// boolean indicates if the cluster exists.
func (c *Client) Select(cluster string, healty bool) (*SocketAddress, bool) {
	if cluster == "" {
		return nil, false
	}
	return c.assignments.Select(cluster, healty)
}

// All returns all endpoints.
func (c *Client) All(cluster string, healty bool) ([]*SocketAddress, []uint32, bool) {
	if cluster == "" {
		return nil, nil, false
	}
	return c.assignments.All(cluster, healty)
}

// Locality holds the locality for this server. It contains a Region, Zone and SubZone.
// Currently this is not used.
type Locality struct {
	Region  string
	Zone    string
	SubZone string
}
