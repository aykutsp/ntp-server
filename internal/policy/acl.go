package policy

import (
	"fmt"
	"net"
)

type CIDRPolicy struct {
	allow []*net.IPNet
	deny  []*net.IPNet
}

func NewCIDRPolicy(allowCIDRs []string, denyCIDRs []string) (*CIDRPolicy, error) {
	allow, err := parseCIDRs(allowCIDRs)
	if err != nil {
		return nil, fmt.Errorf("parse allowCIDRs: %w", err)
	}
	deny, err := parseCIDRs(denyCIDRs)
	if err != nil {
		return nil, fmt.Errorf("parse denyCIDRs: %w", err)
	}
	return &CIDRPolicy{allow: allow, deny: deny}, nil
}

func (p *CIDRPolicy) Allow(ip net.IP) bool {
	if p == nil {
		return true
	}

	for _, block := range p.deny {
		if block.Contains(ip) {
			return false
		}
	}

	if len(p.allow) == 0 {
		return true
	}

	for _, block := range p.allow {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

func parseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, network, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", c, err)
		}
		out = append(out, network)
	}
	return out, nil
}
