/*
 *
 * Copyright 2019 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package client implementation a full fledged gRPC client for the xDS API
// used by the xds resolver and balancer implementations.
package xds

import (
	"errors"
	"fmt"
	"time"

	"github.com/coredns/coredns/plugin/traffic/xds/bootstrap"

	"google.golang.org/grpc"
)

// Options provides all parameters required for the creation of an xDS client.
type Options struct {
	// Config contains a fully populated bootstrap config. It is the
	// responsibility of the caller to use some sane defaults here if the
	// bootstrap process returned with certain fields left unspecified.
	Config bootstrap.Config
	// DialOpts contains dial options to be used when dialing the xDS server.
	DialOpts []grpc.DialOption
}

// Client is a full fledged gRPC client which queries a set of discovery APIs
// (collectively termed as xDS) on a remote management server, to discover
// various dynamic resources. A single client object will be shared by the xds
// resolver and balancer implementations.
type Client struct {
	opts Options
	cc   *grpc.ClientConn // Connection to the xDS server
	v2c  *v2Client        // Actual xDS client implementation using the v2 API

	serviceCallback func(ServiceUpdate, error)
}

// New returns a new xdsClient configured with opts.
func New(opts Options) (*Client, error) {
	switch {
	case opts.Config.BalancerName == "":
		return nil, errors.New("xds: no xds_server name provided in options")
	case opts.Config.Creds == nil:
		fmt.Printf("%s\n", errors.New("xds: no credentials provided in options"))
	case opts.Config.NodeProto == nil:
		return nil, errors.New("xds: no node_proto provided in options")
	}

	var dopts []grpc.DialOption
	if opts.Config.Creds == nil {
		dopts = append([]grpc.DialOption{grpc.WithInsecure()}, opts.DialOpts...)
	} else {
		dopts = append([]grpc.DialOption{opts.Config.Creds}, opts.DialOpts...)
	}
	cc, err := grpc.Dial(opts.Config.BalancerName, dopts...)
	if err != nil {
		// An error from a non-blocking dial indicates something serious.
		return nil, fmt.Errorf("xds: failed to dial balancer {%s}: %v", opts.Config.BalancerName, err)
	}

	println("dialed balancer at", opts.Config.BalancerName)

	c := &Client{
		opts: opts,
		cc:   cc,
		v2c:  newV2Client(cc, opts.Config.NodeProto, func(int) time.Duration { return 0 }),
	}
	return c, nil
}

// Close closes the gRPC connection to the xDS server.
func (c *Client) Close() {
	// TODO: Should we invoke the registered callbacks here with an error that
	// the client is closed?
	c.v2c.close()
	c.cc.Close()
}

func (c *Client) Run() {
	c.v2c.run()
}

// ServiceUpdate contains update about the service.
type ServiceUpdate struct {
	Cluster string
}

// WatchCluster uses CDS to discover information about the provided clusterName.
func (c *Client) WatchCluster(clusterName string, cdsCb func(CDSUpdate, error)) (cancel func()) {
	return c.v2c.watchCDS(clusterName, cdsCb)
}

// WatchEndpoints uses EDS to discover information about the endpoints in a cluster.
func (c *Client) WatchEndpoints(clusterName string, edsCb func(*EDSUpdate, error)) (cancel func()) {
	return c.v2c.watchEDS(clusterName, edsCb)
}
