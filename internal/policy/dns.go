package policy

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type DNSPolicyConfig struct {
	Enabled            bool
	AllowSuffixes      []string
	DenySuffixes       []string
	CacheTTL           time.Duration
	LookupTimeout      time.Duration
	AllowOnLookupError bool
	Nameservers        []string
}

type DNSPolicy struct {
	enabled       bool
	allowSuffixes []string
	denySuffixes  []string
	cacheTTL      time.Duration
	timeout       time.Duration

	allowOnLookupError bool
	resolver           *net.Resolver

	cache sync.Map
	rr    atomic.Uint64
}

type cachedLookup struct {
	expiresAt time.Time
	hosts     []string
	err       string
}

func NewDNSPolicy(cfg DNSPolicyConfig) *DNSPolicy {
	if !cfg.Enabled {
		return &DNSPolicy{enabled: false}
	}

	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.LookupTimeout <= 0 {
		cfg.LookupTimeout = 400 * time.Millisecond
	}

	p := &DNSPolicy{
		enabled:            true,
		allowSuffixes:      normalizeSuffixes(cfg.AllowSuffixes),
		denySuffixes:       normalizeSuffixes(cfg.DenySuffixes),
		cacheTTL:           cfg.CacheTTL,
		timeout:            cfg.LookupTimeout,
		allowOnLookupError: cfg.AllowOnLookupError,
	}

	p.resolver = buildResolver(cfg.Nameservers, cfg.LookupTimeout, &p.rr)
	return p
}

func (p *DNSPolicy) Allow(ctx context.Context, ip net.IP) (bool, string) {
	if p == nil || !p.enabled {
		return true, "disabled"
	}

	key := ip.String()
	now := time.Now()
	if v, ok := p.cache.Load(key); ok {
		cache := v.(cachedLookup)
		if now.Before(cache.expiresAt) {
			return p.evaluate(cache.hosts, cache.err)
		}
		p.cache.Delete(key)
	}

	lctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	hosts, err := p.resolver.LookupAddr(lctx, key)
	var errText string
	if err != nil {
		errText = err.Error()
	}

	normalized := normalizeHosts(hosts)
	p.cache.Store(key, cachedLookup{
		expiresAt: now.Add(p.cacheTTL),
		hosts:     normalized,
		err:       errText,
	})
	return p.evaluate(normalized, errText)
}

func (p *DNSPolicy) evaluate(hosts []string, errText string) (bool, string) {
	if errText != "" {
		if p.allowOnLookupError {
			return true, "lookup error allowed"
		}
		return false, "lookup error denied"
	}

	for _, h := range hosts {
		if hasDomainSuffix(h, p.denySuffixes) {
			return false, "deny suffix match"
		}
	}

	if len(p.allowSuffixes) == 0 {
		return true, "no allow suffix policy"
	}

	for _, h := range hosts {
		if hasDomainSuffix(h, p.allowSuffixes) {
			return true, "allow suffix match"
		}
	}

	return false, "missing allow suffix match"
}

func buildResolver(nameservers []string, timeout time.Duration, rr *atomic.Uint64) *net.Resolver {
	if len(nameservers) == 0 {
		return net.DefaultResolver
	}

	dialer := net.Dialer{
		Timeout: timeout,
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network string, _ string) (net.Conn, error) {
			ns := nameservers[int(rr.Add(1)%uint64(len(nameservers)))]
			return dialer.DialContext(ctx, "udp", ns)
		},
	}
}

func normalizeHosts(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(h, ".")))
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

func normalizeSuffixes(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(strings.Trim(h, ".")))
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

func hasDomainSuffix(host string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
