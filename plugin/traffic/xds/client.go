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
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/coredns/coredns/coremain"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	xdspb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	corepb "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	adsgrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/grpc"
)

var log = clog.NewWithPlugin("traffic: xds")

const (
	cdsURL = "type.googleapis.com/envoy.api.v2.Cluster"
	edsURL = "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"
)

type adsStream adsgrpc.AggregatedDiscoveryService_StreamAggregatedResourcesClient

// Client talks to the grpc manager's endpoint to get load assignments.
type Client struct {
	cc          *grpc.ClientConn
	ctx         context.Context
	assignments *assignment
	node        *corepb.Node
	cancel      context.CancelFunc
	stop        chan struct{}
	mu          sync.RWMutex
	nonce       string
}

// New returns a new client that's dialed to addr using node as the local identifier.
func New(addr, node string, opts ...grpc.DialOption) (*Client, error) {
	cc, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	c := &Client{cc: cc, node: &corepb.Node{Id: node,
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"HOSTNAME": {
					Kind: &structpb.Value_StringValue{StringValue: hostname},
				},
			},
		},
		BuildVersion: coremain.CoreVersion,
	},
	}
	c.assignments = &assignment{cla: make(map[string]*xdspb.ClusterLoadAssignment)}
	c.ctx, c.cancel = context.WithCancel(context.Background())

	return c, nil
}

// Stop stops all goroutines and closes the connection to the upstream manager.
func (c *Client) Stop() error { c.cancel(); return c.cc.Close() }

// Run starts all goroutines and gathers the clusters and endpoint information from the upstream manager.
func (c *Client) Run() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		cli := adsgrpc.NewAggregatedDiscoveryServiceClient(c.cc)
		stream, err := cli.StreamAggregatedResources(c.ctx)
		if err != nil {
			log.Debug(err)
			time.Sleep(2 * time.Second) // grpc's client.go does more spiffy exp. backoff, do we really need that?
			continue
		}

		done := make(chan struct{})
		go func() {
			tick := time.NewTicker(10 * time.Second)
			for {
				select {
				case <-tick.C:
					// send empty list for cluster discovery again and again
					log.Debugf("Requesting cluster list, nonce %q:", c.Nonce())
					if err := c.clusterDiscovery(stream, "", c.Nonce(), []string{}); err != nil {
						log.Debug(err)
					}

				case <-done:
					tick.Stop()
					return
				}
			}
		}()

		if err := c.Receive(stream); err != nil {
			log.Debug(err)
		}
		close(done)
	}
}

// clusterDiscovery sends a cluster DiscoveryRequest on the stream.
func (c *Client) clusterDiscovery(stream adsStream, version, nonce string, clusters []string) error {
	req := &xdspb.DiscoveryRequest{
		Node:          c.node,
		TypeUrl:       cdsURL,
		ResourceNames: clusters, // empty for all
		VersionInfo:   version,
		ResponseNonce: nonce,
	}
	return stream.Send(req)
}

// endpointDiscovery sends a endpoint DiscoveryRequest on the stream.
func (c *Client) endpointDiscovery(stream adsStream, version, nonce string, clusters []string) error {
	req := &xdspb.DiscoveryRequest{
		Node:          c.node,
		TypeUrl:       edsURL,
		ResourceNames: clusters,
		VersionInfo:   version,
		ResponseNonce: nonce,
	}
	return stream.Send(req)
}

// Receive receives from the stream, it handled both cluster and endpoint DiscoveryResponses.
func (c *Client) Receive(stream adsStream) error {
	for {
		resp, err := stream.Recv()
		if err != nil {
			return err
		}

		switch resp.GetTypeUrl() {
		case cdsURL:
			for _, r := range resp.GetResources() {
				var any ptypes.DynamicAny
				if err := ptypes.UnmarshalAny(r, &any); err != nil {
					log.Debugf("Failed to unmarshal cluster discovery: %s", err)
					continue
				}
				cluster, ok := any.Message.(*xdspb.Cluster)
				if !ok {
					continue
				}
				c.assignments.setClusterLoadAssignment(cluster.GetName(), nil)
			}
			log.Debugf("Cluster discovery processed with %d resources", len(resp.GetResources()))

			// ack the CDS proto, with we we've got. (empty version would be NACK)
			if err := c.clusterDiscovery(stream, resp.GetVersionInfo(), resp.GetNonce(), c.assignments.clusters()); err != nil {
				log.Debug(err)
				continue
			}
			// need to figure out how to handle the versions and nounces exactly.

			// now kick off discovery for endpoints
			if err := c.endpointDiscovery(stream, "", resp.GetNonce(), c.assignments.clusters()); err != nil {
				log.Debug(err)
				continue
			}
			c.SetNonce(resp.GetNonce())

		case edsURL:
			for _, r := range resp.GetResources() {
				var any ptypes.DynamicAny
				if err := ptypes.UnmarshalAny(r, &any); err != nil {
					log.Debugf("Failed to unmarshal endpoint discovery: %s", err)
					continue
				}
				cla, ok := any.Message.(*xdspb.ClusterLoadAssignment)
				if !ok {
					continue
				}
				c.assignments.setClusterLoadAssignment(cla.GetClusterName(), cla)
				// ack the bloody thing
			}
			log.Debugf("Endpoint discovery processed with %d resources", len(resp.GetResources()))

		default:
			return fmt.Errorf("unknown response URL for discovery: %q", resp.GetTypeUrl())
		}
	}
}

// Select returns an address that is deemed to be the correct one for this cluster.
func (c *Client) Select(cluster string) net.IP { return c.assignments.Select(cluster) }
