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

package xds

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin/traffic/xds/buffer"

	xdspb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	corepb "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	adsgrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"google.golang.org/grpc"
)

// The value chosen here is based on the default value of the
// initial_fetch_timeout field in corepb.ConfigSource proto.
var defaultWatchExpiryTimeout = 15 * time.Second

// v2Client performs the actual xDS RPCs using the xDS v2 API. It creates a
// single ADS stream on which the different types of xDS requests and responses
// are multiplexed.
// The reason for splitting this out from the top level xdsClient object is
// because there is already an xDS v3Aplha API in development. If and when we
// want to switch to that, this separation will ease that process.
type v2Client struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	// ClientConn to the xDS gRPC server. Owned by the parent xdsClient.
	cc        *grpc.ClientConn
	nodeProto *corepb.Node
	backoff   func(int) time.Duration

	// sendCh in the channel onto which watchInfo objects are pushed by the
	// watch API, and it is read and acted upon by the send() goroutine.
	sendCh *buffer.Unbounded

	mu sync.Mutex
	// Message specific watch infos, protected by the above mutex. These are
	// written to, after successfully reading from the update channel, and are
	// read from when recovering from a broken stream to resend the xDS
	// messages. When the user of this client object cancels a watch call,
	// these are set to nil. All accesses to the map protected and any value
	// inside the map should be protected with the above mutex.
	watchMap map[string]*watchInfo
	// ackMap contains the version that was acked (the version in the ack
	// request that was sent on wire). The key is typeURL, the value is the
	// version string, becaues the versions for different resource types
	// should be independent.
	ackMap map[string]string
	// rdsCache maintains a mapping of {clusterName --> CDSUpdate} from
	// validated cluster configurations received in CDS responses. We cache all
	// valid cluster configurations, whether or not we are interested in them
	// when we received them (because we could become interested in them in the
	// future and the server wont send us those resources again). This is only
	// to support legacy management servers that do not honor the
	// resource_names field. As per the latest spec, the server should resend
	// the response when the request changes, even if it had sent the same
	// resource earlier (when not asked for). Protected by the above mutex.
	cdsCache map[string]CDSUpdate
}

// newV2Client creates a new v2Client initialized with the passed arguments.
func newV2Client(cc *grpc.ClientConn, nodeProto *corepb.Node, backoff func(int) time.Duration) *v2Client {
	v2c := &v2Client{
		cc:        cc,
		nodeProto: nodeProto,
		backoff:   backoff,
		sendCh:    buffer.NewUnbounded(),
		watchMap:  make(map[string]*watchInfo),
		ackMap:    make(map[string]string),
		cdsCache:  make(map[string]CDSUpdate),
	}
	v2c.ctx, v2c.cancelCtx = context.WithCancel(context.Background())

	go v2c.run()
	return v2c
}

// close cleans up resources and goroutines allocated by this client.
func (v2c *v2Client) close() {
	v2c.cancelCtx()
}

// run starts an ADS stream (and backs off exponentially, if the previous
// stream failed without receiving a single reply) and runs the sender and
// receiver routines to send and receive data from the stream respectively.
func (v2c *v2Client) run() {
	retries := 0
	for {
		select {
		case <-v2c.ctx.Done():
			return
		default:
		}

		if retries != 0 {
			t := time.NewTimer(v2c.backoff(retries))
			select {
			case <-t.C:
			case <-v2c.ctx.Done():
				if !t.Stop() {
					<-t.C
				}
				return
			}
		}

		retries++
		cli := adsgrpc.NewAggregatedDiscoveryServiceClient(v2c.cc)
		stream, err := cli.StreamAggregatedResources(v2c.ctx) //, grpc.WaitForReady(true))
		if err != nil {
			log.Infof("xds: ADS stream creation failed: %v", err)
			os.Exit(1)
		}

		// send() could be blocked on reading updates from the different update
		// channels when it is not actually sending out messages. So, we need a
		// way to break out of send() when recv() returns. This done channel is
		// used to for that purpose.
		done := make(chan struct{})
		go v2c.send(stream, done)
		if v2c.recv(stream) {
			retries = 0
		}
		close(done)
	}
}

// sendRequest sends a request for provided typeURL and resource on the provided
// stream.
//
// version is the ack version to be sent with the request
// - If this is the new request (not an ack/nack), version will be an empty
// string
// - If this is an ack, version will be the version from the response
// - If this is a nack, version will be the previous acked version (from
// ackMap). If there was no ack before, it will be an empty string
func (v2c *v2Client) sendRequest(stream adsStream, resourceNames []string, typeURL, version, nonce string) bool {
	req := &xdspb.DiscoveryRequest{
		Node:          v2c.nodeProto,
		TypeUrl:       typeURL,
		ResourceNames: resourceNames,
		VersionInfo:   version,
		ResponseNonce: nonce,
		// TODO: populate ErrorDetails for nack.
	}
	println("v2: sendrequest", typeURL)
	if err := stream.Send(req); err != nil {
		log.Warningf("xds: request (type %s) for resource %v failed: %v", typeURL, resourceNames, err)
		return false
	}
	return true
}

// sendExisting sends out xDS requests for registered watchers when recovering
// from a broken stream.
//
// We call stream.Send() here with the lock being held. It should be OK to do
// that here because the stream has just started and Send() usually returns
// quickly (once it pushes the message onto the transport layer) and is only
// ever blocked if we don't have enough flow control quota.
func (v2c *v2Client) sendExisting(stream adsStream) bool {
	println("v2: sendexisting")
	v2c.mu.Lock()
	defer v2c.mu.Unlock()

	// Reset the ack versions when the stream restarts.
	v2c.ackMap = make(map[string]string)

	for typeURL, wi := range v2c.watchMap {
		if !v2c.sendRequest(stream, wi.target, typeURL, "", "") {
			return false
		}
	}

	return true
}

// processWatchInfo pulls the fields needed by the request from a watchInfo.
//
// It also calls callback with cached response, and updates the watch map in
// v2c.
//
// If the watch was already canceled, it returns false for send
func (v2c *v2Client) processWatchInfo(t *watchInfo) (target []string, typeURL, version, nonce string, send bool) {
	v2c.mu.Lock()
	defer v2c.mu.Unlock()
	if t.state == watchCancelled {
		return // This returns all zero values, and false for send.
	}
	t.state = watchStarted
	send = true

	typeURL = t.typeURL
	target = t.target
	v2c.checkCacheAndUpdateWatchMap(t)
	// TODO: if watch is called again with the same resource names,
	// there's no need to send another request.
	//
	// TODO: should we reset version (for ack) when a new watch is
	// started? Or do this only if the resource names are different
	// (so we send a new request)?
	return
}

// processAckInfo pulls the fields needed by the ack request from a ackInfo.
//
// If no active watch is found for this ack, it returns false for send.
func (v2c *v2Client) processAckInfo(t *ackInfo) (target []string, typeURL, version, nonce string, send bool) {
	typeURL = t.typeURL

	v2c.mu.Lock()
	defer v2c.mu.Unlock()
	wi, ok := v2c.watchMap[typeURL]
	if !ok {
		// We don't send the request ack if there's no active watch (this can be
		// either the server sends responses before any request, or the watch is
		// canceled while the ackInfo is in queue), because there's no resource
		// name. And if we send a request with empty resource name list, the
		// server may treat it as a wild card and send us everything.
		log.Warningf("xds: ack (type %s) not sent because there's no active watch for the type", typeURL)
		return // This returns all zero values, and false for send.
	}
	send = true

	version = t.version
	nonce = t.nonce
	target = wi.target
	if version == "" {
		// This is a nack, get the previous acked version.
		version = v2c.ackMap[typeURL]
		// version will still be an empty string if typeURL isn't
		// found in ackMap, this can happen if there wasn't any ack
		// before.
	} else {
		v2c.ackMap[typeURL] = version
	}
	return
}

// send reads watch infos from update channel and sends out actual xDS requests
// on the provided ADS stream.
func (v2c *v2Client) send(stream adsStream, done chan struct{}) {
	if !v2c.sendExisting(stream) {
		println("not existing stream")
		return
	}

	println("in send")

	for {
		select {
		case <-v2c.ctx.Done():
			return
		case u := <-v2c.sendCh.Get():
			v2c.sendCh.Load()

			var (
				target                  []string
				typeURL, version, nonce string
				send                    bool
			)
			switch t := u.(type) {
			case *watchInfo:
				println("watchInfo")
				target, typeURL, version, nonce, send = v2c.processWatchInfo(t)
				println(target, typeURL, version, nonce, send)
				fmt.Printf("%+v\n", target)
			case *ackInfo:
				println("ackInfo")
				target, typeURL, version, nonce, send = v2c.processAckInfo(t)
			}
			if !send {
				continue
			}
			if !v2c.sendRequest(stream, target, typeURL, version, nonce) {
				return
			}
		case <-done:
			return
		}
	}
}

// recv receives xDS responses on the provided ADS stream and branches out to
// message specific handlers.
func (v2c *v2Client) recv(stream adsStream) bool {
	println("v2 recv")
	success := false
	for {
		println("WATIIGNM")
		resp, err := stream.Recv()
		// TODO: call watch callbacks with error when stream is broken.
		println("DONE")
		if err != nil {
			log.Warningf("xds: ADS stream recv failed: %v", err)
			return success
		}
		println("RECEIVING")
		var respHandleErr error
		switch resp.GetTypeUrl() {
		case cdsURL:
			println("CDS")
			respHandleErr = v2c.handleCDSResponse(resp)
		case edsURL:
			println("EDS")
			respHandleErr = v2c.handleEDSResponse(resp)
		default:
			log.Warningf("xds: unknown response URL type: %v", resp.GetTypeUrl())
			continue
		}

		typeURL := resp.GetTypeUrl()
		if respHandleErr != nil {
			log.Warningf("xds: response (type %s) handler failed: %v", typeURL, respHandleErr)
			v2c.sendCh.Put(&ackInfo{
				typeURL: typeURL,
				version: "",
				nonce:   resp.GetNonce(),
			})
			continue
		}
		v2c.sendCh.Put(&ackInfo{
			typeURL: typeURL,
			version: resp.GetVersionInfo(),
			nonce:   resp.GetNonce(),
		})
		success = true
	}
}

// watchCDS registers an CDS watcher for the provided clusterName. Updates
// corresponding to received CDS responses will be pushed to the provided
// callback. The caller can cancel the watch by invoking the returned cancel
// function.
// The provided callback should not block or perform any expensive operations
// or call other methods of the v2Client object.
func (v2c *v2Client) watchCDS(clusterName string, cdsCb cdsCallback) (cancel func()) {
	return v2c.watch(&watchInfo{
		typeURL:  cdsURL,
		target:   []string{clusterName},
		callback: cdsCb,
	})
}

// watchEDS registers an EDS watcher for the provided clusterName. Updates
// corresponding to received EDS responses will be pushed to the provided
// callback. The caller can cancel the watch by invoking the returned cancel
// function.
// The provided callback should not block or perform any expensive operations
// or call other methods of the v2Client object.
func (v2c *v2Client) watchEDS(clusterName string, edsCb edsCallback) (cancel func()) {
	return v2c.watch(&watchInfo{
		typeURL:  edsURL,
		target:   []string{clusterName},
		callback: edsCb,
	})
	// TODO: Once a registered EDS watch is cancelled, we should send an EDS
	// request with no resources. This will let the server know that we are no
	// longer interested in this resource.
}

func (v2c *v2Client) watch(wi *watchInfo) (cancel func()) {
	v2c.sendCh.Put(wi)
	return func() {
		v2c.mu.Lock()
		defer v2c.mu.Unlock()
		if wi.state == watchEnqueued {
			wi.state = watchCancelled
			return
		}
		v2c.watchMap[wi.typeURL].cancel()
		delete(v2c.watchMap, wi.typeURL)
		// TODO: should we reset ack version string when cancelling the watch?
	}
}

// checkCacheAndUpdateWatchMap is called when a new watch call is handled in
// send(). If an existing watcher is found, its expiry timer is stopped. If the
// watchInfo to be added to the watchMap is found in the cache, the watcher
// callback is immediately invoked.
//
// Caller should hold v2c.mu
func (v2c *v2Client) checkCacheAndUpdateWatchMap(wi *watchInfo) {
	if existing := v2c.watchMap[wi.typeURL]; existing != nil {
		println("cancel")
		existing.cancel()
	}

	v2c.watchMap[wi.typeURL] = wi
	switch wi.typeURL {
	// We need to grab the lock inside of the expiryTimer's afterFunc because
	// we need to access the watchInfo, which is stored in the watchMap.
	case cdsURL:
		clusterName := wi.target[0]
		println("CDS URLS", clusterName)
		if update, ok := v2c.cdsCache[clusterName]; ok {
			println("UPDATE SEEN, ok")

			var err error
			if v2c.watchMap[cdsURL] == nil {
				err = fmt.Errorf("xds: no CDS watcher found when handling CDS watch for cluster {%v} from cache", clusterName)
			}
			wi.callback.(cdsCallback)(update, err)
			return
		}
		wi.expiryTimer = time.AfterFunc(defaultWatchExpiryTimeout, func() {
			v2c.mu.Lock()
			wi.callback.(cdsCallback)(CDSUpdate{}, fmt.Errorf("xds: CDS target %s not found", wi.target))
			v2c.mu.Unlock()
		})
	case edsURL:
		wi.expiryTimer = time.AfterFunc(defaultWatchExpiryTimeout, func() {
			v2c.mu.Lock()
			wi.callback.(edsCallback)(nil, fmt.Errorf("xds: EDS target %s not found", wi.target))
			v2c.mu.Unlock()
		})
	}
}
