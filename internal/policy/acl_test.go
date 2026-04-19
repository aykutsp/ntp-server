package policy

import (
	"net"
	"testing"
)

func TestCIDRPolicy(t *testing.T) {
	p, err := NewCIDRPolicy(
		[]string{"10.0.0.0/8"},
		[]string{"10.10.0.0/16"},
	)
	if err != nil {
		t.Fatalf("policy setup failed: %v", err)
	}

	if !p.Allow(net.ParseIP("10.20.1.4")) {
		t.Fatalf("10.20.1.4 should be allowed")
	}
	if p.Allow(net.ParseIP("10.10.2.4")) {
		t.Fatalf("10.10.2.4 should be denied by deny rule")
	}
	if p.Allow(net.ParseIP("192.168.1.1")) {
		t.Fatalf("192.168.1.1 should be denied because allow list exists")
	}
}
