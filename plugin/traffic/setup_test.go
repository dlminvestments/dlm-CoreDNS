package traffic

import (
	"encoding/json"
	"testing"

	"github.com/caddyserver/caddy"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `traffic grpc://127.0.0.1`)
	if err := setup(c); err != nil {
		t.Fatalf("Expected no errors, but got: %q", err)
	}
}

func TestLBTxt(t *testing.T) {
	_, err := json.Marshal(lbTXT)
	if err != nil {
		t.Fatalf("Failed to marshal grpc serverConfig: %s", err)
	}
	if len(lbTXT) > 255 {
		t.Fatalf("Too long grpc serverConfig (>255): %d", len(lbTXT))
	}
}

func TestParseTraffic(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
	}{
		// ok
		{`traffic grpc://127.0.0.1:18000 {
			id test-id
		}`, false},

		// fail
		{`traffic`, true},
		{`traffic tls://1.1.1.1`, true},
		{`traffic {
			id bla bla
		}`, true},
		{`traffic {
			node
		}`, true},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		_, err := parseTraffic(c)
		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error, but got: nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %d: Expected no error, but got: %v", i, err)
			continue
		}

		if test.shouldErr {
			continue
		}
	}
}
