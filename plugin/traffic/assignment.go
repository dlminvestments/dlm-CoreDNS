package traffic

import (
	"math/rand"
	"net"
)

// assignment is an assignment for a single service. It contains multiple backends.
type assignment struct {
	service  string
	backends []*backend
}

// backend is a backend specified by an address, port and a weight.
type backend struct {
	addr   net.IP
	port   int
	weight int
}

// Select selects a backend from a, using weighted random selection
func (a assignment) Select() *backend {
	total := 0
	for _, b := range a.backends {
		total += b.weight
	}
	if total == 0 {
		return nil
	}
	r := rand.Intn(total) + 1

	for _, b := range a.backends {
		r -= b.weight
		if r <= 0 {
			return b
		}
	}
	return nil
}
