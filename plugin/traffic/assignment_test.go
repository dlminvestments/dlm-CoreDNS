package traffic

import (
	"math/rand"
	"net"
	"testing"
	"time"
)

func TestAssignment(t *testing.T) {
	rand.Seed(int64(time.Now().Nanosecond()))

	backends := []*backend{
		{net.IPv4zero, 0, 6},
		{net.IPv4allrouter, 0, 4},
		{net.IPv4allsys, 0, 0},
	}
	a := assignment{"www.example.org", backends}

	// should never get 0 weight, could be improved to check the difference between 4 and 6.
	for i := 0; i < 100; i++ {
		if x := a.Select(); x.weight == 0 {
			t.Errorf("Expected non-nil weight for Select, got %v", x)
		}
	}
}

func TestAssignmentZero(t *testing.T) {
	rand.Seed(int64(time.Now().Nanosecond()))

	backends := []*backend{
		{net.IPv4zero, 0, 0},
	}
	a := assignment{"www.example.org", backends}
	if x := a.Select(); x != nil {
		t.Errorf("Expected nil for Select, got %v", x)
	}
}
