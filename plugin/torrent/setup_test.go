package torrent

import (
	"testing"

	"github.com/caddyserver/caddy"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input     string
		shouldErr bool
		exp       *Torrent
	}{
		{`torrent testdata/db.miek.nl {
			seed
		 }`,
			false,
			&Torrent{dbfile: "testdata/db.miek.nl", seed: true},
		},
		{`torrent testdata/db.miek.nl`,
			false,
			&Torrent{dbfile: "testdata/db.miek.nl"},
		},
		// errors
		{`torrent db.example.org {
			bla
		 }`,
			true,
			nil,
		},
	}
	for i, tc := range tests {
		c := caddy.NewTestController("dns", tc.input)
		tor, err := parse(c)

		if err == nil && tc.shouldErr {
			t.Fatalf("Test %d expected errors, but got no error", i)
		}
		if err != nil && !tc.shouldErr {
			t.Fatalf("Test %d expected no errors, but got '%v'", i, err)
		}
		if tc.shouldErr {
			continue
		}
		if x := tor.dbfile; x != tc.exp.dbfile {
			t.Errorf("Test %d expected %s as dbfile, got %s", i, tc.exp.dbfile, x)
		}
		if x := tor.seed; x != tc.exp.seed {
			t.Errorf("Test %d expected %T as seed, got %T", i, tc.exp.seed, x)
		}
	}
}
