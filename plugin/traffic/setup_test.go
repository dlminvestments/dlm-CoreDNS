package traffic

import (
	"testing"

	"github.com/caddyserver/caddy"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `traffic grpc://127.0.0.1`)
	if err := setup(c); err != nil {
		t.Fatalf("Test 1, expected no errors, but got: %q", err)
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
			t.Errorf("Test %v: Expected error but found nil", i)
			continue
		} else if !test.shouldErr && err != nil {
			t.Errorf("Test %v: Expected no error but found error: %v", i, err)
			continue
		}

		if test.shouldErr {
			continue
		}
	}
}

func testParseLocality(t *testing.T) {
	s := "region"
	locs, err := parseLocality(s)
	if err != nil {
		t.Fatal(err)
	}
	if locs[0].Region != "region" {
		t.Errorf("Expected %s, but got %s", "region", locs[0].Region)
	}

	s = "region1,zone,sub region2"
	locs, err = parseLocality(s)
	if err != nil {
		t.Fatal(err)
	}
	if locs[0].Zone != "zone" {
		t.Errorf("Expected %s, but got %s", "zone", locs[1].Zone)
	}
	if locs[0].SubZone != "sub" {
		t.Errorf("Expected %s, but got %s", "sub", locs[1].SubZone)
	}
	if locs[1].Region != "region2" {
		t.Errorf("Expected %s, but got %s", "region2", locs[1].Region)
	}
	if locs[1].Zone != "" {
		t.Errorf("Expected %s, but got %s", "", locs[1].Zone)
	}
}
