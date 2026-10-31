package wireguard

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestWaitForLinkGivesUpAfterTheTimeout(t *testing.T) {
	const timeout = 300 * time.Millisecond

	start := time.Now()
	err := waitForLink("gate-none0", timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("waitForLink returned nil for a link that never appears")
	}
	if elapsed < timeout {
		t.Errorf("waitForLink gave up after %s, want at least %s", elapsed, timeout)
	}
	if !strings.Contains(err.Error(), "ensure a userspace implementation") {
		t.Errorf("error %q lacks the actionable hint", err)
	}
}

func TestGetIP(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "host route", in: "10.8.0.2/32", want: "10.8.0.2/32"},
		{name: "not an address", in: "not-an-ip/32", wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "missing prefix", in: "10.8.0.2", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getIP(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getIP(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != 1 || got[0].String() != tt.want {
				t.Errorf("getIP(%q) = %v, want [%s]", tt.in, got, tt.want)
			}
		})
	}
}

func TestAllowedIPsEqual(t *testing.T) {
	mustNets := func(t *testing.T, cidrs ...string) []net.IPNet {
		t.Helper()
		var nets []net.IPNet
		for _, c := range cidrs {
			_, n, err := net.ParseCIDR(c)
			if err != nil {
				t.Fatalf("ParseCIDR(%q): %v", c, err)
			}
			nets = append(nets, *n)
		}
		return nets
	}

	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{name: "both empty", want: true},
		{name: "same", a: []string{"10.8.0.2/32"}, b: []string{"10.8.0.2/32"}, want: true},
		{name: "different address", a: []string{"10.8.0.2/32"}, b: []string{"10.8.0.3/32"}},
		{name: "different mask", a: []string{"10.8.0.0/24"}, b: []string{"10.8.0.0/32"}},
		{name: "different length", a: []string{"10.8.0.2/32"}},
		{name: "device peer with no allowed IPs", b: []string{"10.8.0.2/32"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowedIPsEqual(mustNets(t, tt.a...), mustNets(t, tt.b...)); got != tt.want {
				t.Errorf("allowedIPsEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
