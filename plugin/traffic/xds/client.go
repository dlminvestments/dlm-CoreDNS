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
	node        *corepb.Node
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
	hostname, _ := os.Hostname()
	c := &Client{cc: cc, to: addr, node: &corepb.Node{Id: node,
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
	c.version, c.nonce = make(map[string]string), make(map[string]string)
	c.ctx, c.cancel = context.WithCancel(context.Background())

	return c, nil
}

// Stop stops all goroutines and closes the connection to the upstream manager.
func (c *Client) Stop() error { c.cancel(); return c.cc.Close() }

// Run starts all goroutines and gathers the clusters and endpoint information from the upstream manager.
func (c *Client) Run() {
	first := true
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

		if first {
			log.Infof("gRPC stream established to %q", c.to)
			c.setSynced()
			first = false
		}

		done := make(chan struct{})
		go func() {
			if err := c.clusterDiscovery(stream, c.Version(cdsURL), c.Nonce(cdsURL), []string{}); err != nil {
				log.Debug(err)
			}
			tick := time.NewTicker(10 * time.Second)
			for {
				select {
				case <-tick.C:
					// send empty list for cluster discovery every 10 seconds
					if err := c.clusterDiscovery(stream, c.Version(cdsURL), c.Nonce(cdsURL), []string{}); err != nil {
						log.Debug(err)
					}

				case <-done:
					tick.Stop()
					return
				}
			}
		}()

		if err := c.receive(stream); err != nil {
			log.Warning(err)
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

// receive receives from the stream, it handled both cluster and endpoint DiscoveryResponses.
func (c *Client) receive(stream adsStream) error {
	for {
		resp, err := stream.Recv()
		if err != nil {
			return err
		}

		switch resp.GetTypeUrl() {
		case cdsURL:
			a := NewAssignment()
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
				a.SetClusterLoadAssignment(cluster.GetName(), nil)
			}
			log.Debugf("Cluster discovery processed with %d resources, version %q and nonce %q", len(resp.GetResources()), c.Version(cdsURL), c.Nonce(cdsURL))
			ClusterGauge.Set(float64(len(resp.GetResources())))
			// set our local administration and ack the reply. Empty version would signal NACK.
			c.SetNonce(cdsURL, resp.GetNonce())
			c.SetVersion(cdsURL, resp.GetVersionInfo())
			c.SetAssignments(a)
			c.clusterDiscovery(stream, resp.GetVersionInfo(), resp.GetNonce(), a.clusters())

			// now kick off discovery for endpoints
			if err := c.endpointDiscovery(stream, c.Version(edsURL), c.Nonce(edsURL), a.clusters()); err != nil {
				log.Debug(err)
			}
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
				c.assignments.SetClusterLoadAssignment(cla.GetClusterName(), cla)
			}
			log.Debugf("Endpoint discovery processed with %d resources, version %q and nonce %q", len(resp.GetResources()), c.Version(edsURL), c.Nonce(edsURL))
			// set our local administration and ack the reply. Empty version would signal NACK.
			c.SetNonce(edsURL, resp.GetNonce())
			c.SetVersion(edsURL, resp.GetVersionInfo())

		default:
			return fmt.Errorf("unknown response URL for discovery: %q", resp.GetTypeUrl())
		}
	}
}

// Select returns an address that is deemed to be the correct one for this cluster. The returned
// boolean indicates if the cluster exists.
func (c *Client) Select(cluster string, locality []Locality, ignore bool) (*SocketAddress, bool) {
	if cluster == "" {
		return nil, false
	}
	return c.assignments.Select(cluster, locality, ignore)
}

// All returns all endpoints.
func (c *Client) All(cluster string, locality []Locality, ignore bool) ([]*SocketAddress, bool) {
	if cluster == "" {
		return nil, false
	}
	return c.assignments.All(cluster, locality, ignore)
}

// Locality holds the locality for this server. It contains a Region, Zone and SubZone.
type Locality struct {
	Region  string
	Zone    string
	SubZone string
}
