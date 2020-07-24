package xds

import (
	"math/rand"
	"net"
	"sync"

	xdspb2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	corepb2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

// SocketAddress holds a corepb2.SocketAddress and a health status
type SocketAddress struct {
	*corepb2.SocketAddress
	Health corepb2.HealthStatus
}

// Address returns the address from s.
func (s *SocketAddress) Address() net.IP { return net.ParseIP(s.GetAddress()) }

// Port returns the port from s.
func (s *SocketAddress) Port() uint16 { return uint16(s.GetPortValue()) }

type assignment struct {
	mu  sync.RWMutex
	cla map[string]*xdspb2.ClusterLoadAssignment
}

// NewAssignment returns a pointer to an assignment.
func NewAssignment() *assignment {
	return &assignment{cla: make(map[string]*xdspb2.ClusterLoadAssignment)}
}

// SetClusterLoadAssignment sets the assignment for the cluster to cla.
func (a *assignment) SetClusterLoadAssignment(cluster string, cla *xdspb2.ClusterLoadAssignment) {
	// If cla is nil we just found a cluster, check if we already know about it, or if we need to make a new entry.
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.cla[cluster]
	if !ok {
		a.cla[cluster] = cla
		return
	}
	if cla == nil {
		return
	}
	a.cla[cluster] = cla

}

// ClusterLoadAssignment returns the assignment for the cluster or nil if there is none.
func (a *assignment) ClusterLoadAssignment(cluster string) *xdspb2.ClusterLoadAssignment {
	a.mu.RLock()
	cla, ok := a.cla[cluster]
	a.mu.RUnlock()
	if !ok {
		return nil
	}
	return cla
}

func (a *assignment) clusters() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	clusters := make([]string, len(a.cla))
	i := 0
	for k := range a.cla {
		clusters[i] = k
		i++
	}
	return clusters
}

// Select selects a endpoint from cluster load assignments, using weighted random selection. It only selects endpoints that are reporting healthy.
func (a *assignment) Select(cluster string, healthy bool) (*SocketAddress, bool) {
	cla := a.ClusterLoadAssignment(cluster)
	if cla == nil {
		return nil, false
	}

	weight := 0
	health := 0
	for _, ep := range cla.Endpoints {
		for _, lb := range ep.GetLbEndpoints() {
			if healthy && lb.GetHealthStatus() != corepb2.HealthStatus_HEALTHY {
				continue
			}
			weight += int(lb.GetLoadBalancingWeight().GetValue())
			health++
		}
	}
	if health == 0 {
		return nil, true
	}

	// all weights are 0, randomly select one of the endpoints,
	if weight == 0 {
		r := rand.Intn(health)
		i := 0
		for _, ep := range cla.Endpoints {
			for _, lb := range ep.GetLbEndpoints() {
				if healthy && lb.GetHealthStatus() != corepb2.HealthStatus_HEALTHY {
					continue
				}
				if r == i {
					return &SocketAddress{lb.GetEndpoint().GetAddress().GetSocketAddress(), lb.GetHealthStatus()}, true
				}
				i++
			}
		}
		return nil, true
	}

	r := rand.Intn(health) + 1
	for _, ep := range cla.Endpoints {
		for _, lb := range ep.GetLbEndpoints() {
			if healthy && lb.GetHealthStatus() != corepb2.HealthStatus_HEALTHY {
				continue
			}
			r -= int(lb.GetLoadBalancingWeight().GetValue())
			if r <= 0 {
				return &SocketAddress{lb.GetEndpoint().GetAddress().GetSocketAddress(), lb.GetHealthStatus()}, true
			}
		}
	}
	return nil, true
}

// All returns all healthy endpoints, together with their weights.
func (a *assignment) All(cluster string, healthy bool) ([]*SocketAddress, []uint32, bool) {
	cla := a.ClusterLoadAssignment(cluster)
	if cla == nil {
		return nil, nil, false
	}

	sa := []*SocketAddress{}
	we := []uint32{}
	for _, ep := range cla.Endpoints {
		for _, lb := range ep.GetLbEndpoints() {
			if healthy && lb.GetHealthStatus() != corepb2.HealthStatus_HEALTHY {
				continue
			}
			weight := lb.GetLoadBalancingWeight().GetValue()
			if weight > 2^16 {
				log.Warning("Weight in cluster %q > %d, truncating to %d in SRV responses", cluster, weight, uint16(weight))
			}
			we = append(we, weight)
			sa = append(sa, &SocketAddress{lb.GetEndpoint().GetAddress().GetSocketAddress(), lb.GetHealthStatus()})
		}
	}
	return sa, we, true
}
