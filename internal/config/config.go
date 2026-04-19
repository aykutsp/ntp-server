package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

type Config struct {
	NTP      NTPConfig      `json:"ntp"`
	Policy   PolicyConfig   `json:"policy"`
	Upstream UpstreamConfig `json:"upstream"`
	API      APIConfig      `json:"api"`
	Runtime  RuntimeConfig  `json:"runtime"`
}

type NTPConfig struct {
	ListenAddress             string  `json:"listenAddress"`
	Workers                   int     `json:"workers"`
	ReadBufferBytes           int     `json:"readBufferBytes"`
	WriteBufferBytes          int     `json:"writeBufferBytes"`
	ReadTimeoutMillis         int     `json:"readTimeoutMillis"`
	WriteTimeoutMillis        int     `json:"writeTimeoutMillis"`
	ClientRateLimitPerSecond  float64 `json:"clientRateLimitPerSecond"`
	ClientRateLimitBurst      int     `json:"clientRateLimitBurst"`
	GlobalRateLimitPerSecond  float64 `json:"globalRateLimitPerSecond"`
	GlobalRateLimitBurst      int     `json:"globalRateLimitBurst"`
	ServeUnsynced             bool    `json:"serveUnsynced"`
	UnsyncedStratum           uint8   `json:"unsyncedStratum"`
	ReferenceID               string  `json:"referenceID"`
	PrecisionExponent         int8    `json:"precisionExponent"`
	RootDelayMillis           float64 `json:"rootDelayMillis"`
	RootDispersionMillis      float64 `json:"rootDispersionMillis"`
	EnableKissOfDeathOnDeny   bool    `json:"enableKissOfDeathOnDeny"`
	KissOfDeathResponseOnDeny string  `json:"kissOfDeathResponseOnDeny"`
}

type PolicyConfig struct {
	AllowCIDRs []string        `json:"allowCIDRs"`
	DenyCIDRs  []string        `json:"denyCIDRs"`
	DNS        DNSPolicyConfig `json:"dns"`
}

type DNSPolicyConfig struct {
	EnableReverseLookup bool     `json:"enableReverseLookup"`
	AllowSuffixes       []string `json:"allowSuffixes"`
	DenySuffixes        []string `json:"denySuffixes"`
	CacheTTLSeconds     int      `json:"cacheTTLSeconds"`
	LookupTimeoutMillis int      `json:"lookupTimeoutMillis"`
	AllowOnLookupError  bool     `json:"allowOnLookupError"`
	ResolverNameservers []string `json:"resolverNameservers"`
}

type UpstreamConfig struct {
	Enabled                 bool     `json:"enabled"`
	Servers                 []string `json:"servers"`
	SyncIntervalSeconds     int      `json:"syncIntervalSeconds"`
	TimeoutMillis           int      `json:"timeoutMillis"`
	MaxAcceptedOffsetMillis int      `json:"maxAcceptedOffsetMillis"`
	SamplesPerSync          int      `json:"samplesPerSync"`
}

type APIConfig struct {
	Enabled              bool   `json:"enabled"`
	ListenAddress        string `json:"listenAddress"`
	ReadTimeoutMillis    int    `json:"readTimeoutMillis"`
	WriteTimeoutMillis   int    `json:"writeTimeoutMillis"`
	ShutdownTimeoutMilli int    `json:"shutdownTimeoutMillis"`
	AuthToken            string `json:"authToken"`
	ExposeConfig         bool   `json:"exposeConfig"`
}

type RuntimeConfig struct {
	GOMAXPROCS int    `json:"gomaxprocs"`
	LogLevel   string `json:"logLevel"`
}

func Default() Config {
	workers := runtime.NumCPU() * 4
	if workers < 8 {
		workers = 8
	}

	return Config{
		NTP: NTPConfig{
			ListenAddress:             "0.0.0.0:12300",
			Workers:                   workers,
			ReadBufferBytes:           32 * 1024 * 1024,
			WriteBufferBytes:          32 * 1024 * 1024,
			ReadTimeoutMillis:         1000,
			WriteTimeoutMillis:        1000,
			ClientRateLimitPerSecond:  32,
			ClientRateLimitBurst:      64,
			GlobalRateLimitPerSecond:  400000,
			GlobalRateLimitBurst:      100000,
			ServeUnsynced:             true,
			UnsyncedStratum:           16,
			ReferenceID:               "LOCL",
			PrecisionExponent:         -20,
			RootDelayMillis:           1.0,
			RootDispersionMillis:      1.0,
			EnableKissOfDeathOnDeny:   true,
			KissOfDeathResponseOnDeny: "RATE",
		},
		Policy: PolicyConfig{
			AllowCIDRs: []string{},
			DenyCIDRs:  []string{},
			DNS: DNSPolicyConfig{
				EnableReverseLookup: false,
				AllowSuffixes:       []string{},
				DenySuffixes:        []string{},
				CacheTTLSeconds:     300,
				LookupTimeoutMillis: 400,
				AllowOnLookupError:  true,
				ResolverNameservers: []string{},
			},
		},
		Upstream: UpstreamConfig{
			Enabled:                 true,
			Servers:                 []string{"time.cloudflare.com:123", "time.google.com:123", "pool.ntp.org:123"},
			SyncIntervalSeconds:     10,
			TimeoutMillis:           1200,
			MaxAcceptedOffsetMillis: 1500,
			SamplesPerSync:          1,
		},
		API: APIConfig{
			Enabled:              true,
			ListenAddress:        "127.0.0.1:8080",
			ReadTimeoutMillis:    2000,
			WriteTimeoutMillis:   2000,
			ShutdownTimeoutMilli: 5000,
			AuthToken:            "",
			ExposeConfig:         false,
		},
		Runtime: RuntimeConfig{
			GOMAXPROCS: runtime.NumCPU(),
			LogLevel:   "INFO",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		cfg.Normalize()
		return cfg, cfg.Validate()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) Normalize() {
	c.NTP.ListenAddress = strings.TrimSpace(c.NTP.ListenAddress)
	if c.NTP.Workers <= 0 {
		c.NTP.Workers = runtime.NumCPU() * 4
	}
	if c.NTP.ReadTimeoutMillis <= 0 {
		c.NTP.ReadTimeoutMillis = 1000
	}
	if c.NTP.WriteTimeoutMillis <= 0 {
		c.NTP.WriteTimeoutMillis = 1000
	}
	if c.NTP.ClientRateLimitPerSecond > 0 && c.NTP.ClientRateLimitBurst <= 0 {
		c.NTP.ClientRateLimitBurst = int(c.NTP.ClientRateLimitPerSecond * 2)
	}
	if c.NTP.GlobalRateLimitPerSecond > 0 && c.NTP.GlobalRateLimitBurst <= 0 {
		c.NTP.GlobalRateLimitBurst = int(c.NTP.GlobalRateLimitPerSecond / 4)
	}
	if c.NTP.ReferenceID == "" {
		c.NTP.ReferenceID = "LOCL"
	}
	if c.NTP.KissOfDeathResponseOnDeny == "" {
		c.NTP.KissOfDeathResponseOnDeny = "RATE"
	}

	c.Policy.DNS.AllowSuffixes = normalizeSuffixes(c.Policy.DNS.AllowSuffixes)
	c.Policy.DNS.DenySuffixes = normalizeSuffixes(c.Policy.DNS.DenySuffixes)
	if c.Policy.DNS.CacheTTLSeconds <= 0 {
		c.Policy.DNS.CacheTTLSeconds = 300
	}
	if c.Policy.DNS.LookupTimeoutMillis <= 0 {
		c.Policy.DNS.LookupTimeoutMillis = 400
	}
	c.Policy.DNS.ResolverNameservers = normalizeNameservers(c.Policy.DNS.ResolverNameservers)

	if c.Upstream.SyncIntervalSeconds <= 0 {
		c.Upstream.SyncIntervalSeconds = 10
	}
	if c.Upstream.TimeoutMillis <= 0 {
		c.Upstream.TimeoutMillis = 1200
	}
	if c.Upstream.MaxAcceptedOffsetMillis <= 0 {
		c.Upstream.MaxAcceptedOffsetMillis = 1500
	}
	if c.Upstream.SamplesPerSync <= 0 {
		c.Upstream.SamplesPerSync = 1
	}
	for i := range c.Upstream.Servers {
		c.Upstream.Servers[i] = normalizeNTPServer(c.Upstream.Servers[i])
	}

	c.API.ListenAddress = strings.TrimSpace(c.API.ListenAddress)
	if c.API.ReadTimeoutMillis <= 0 {
		c.API.ReadTimeoutMillis = 2000
	}
	if c.API.WriteTimeoutMillis <= 0 {
		c.API.WriteTimeoutMillis = 2000
	}
	if c.API.ShutdownTimeoutMilli <= 0 {
		c.API.ShutdownTimeoutMilli = 5000
	}

	c.Runtime.LogLevel = strings.ToUpper(strings.TrimSpace(c.Runtime.LogLevel))
	if c.Runtime.LogLevel == "" {
		c.Runtime.LogLevel = "INFO"
	}
	if c.Runtime.GOMAXPROCS <= 0 {
		c.Runtime.GOMAXPROCS = runtime.NumCPU()
	}
}

func (c Config) Validate() error {
	if c.NTP.ListenAddress == "" {
		return errors.New("ntp.listenAddress is required")
	}
	if c.NTP.Workers <= 0 {
		return errors.New("ntp.workers must be > 0")
	}
	if c.NTP.ClientRateLimitPerSecond < 0 {
		return errors.New("ntp.clientRateLimitPerSecond cannot be negative")
	}
	if c.NTP.GlobalRateLimitPerSecond < 0 {
		return errors.New("ntp.globalRateLimitPerSecond cannot be negative")
	}
	if c.NTP.UnsyncedStratum > 16 {
		return errors.New("ntp.unsyncedStratum must be <= 16")
	}
	if len(c.NTP.ReferenceID) > 4 {
		return errors.New("ntp.referenceID must be max 4 chars")
	}
	if c.Policy.DNS.CacheTTLSeconds < 1 {
		return errors.New("policy.dns.cacheTTLSeconds must be >= 1")
	}
	if c.Policy.DNS.LookupTimeoutMillis < 1 {
		return errors.New("policy.dns.lookupTimeoutMillis must be >= 1")
	}
	if c.Upstream.Enabled && len(c.Upstream.Servers) == 0 {
		return errors.New("upstream.servers cannot be empty when upstream is enabled")
	}
	if c.API.Enabled && c.API.ListenAddress == "" {
		return errors.New("api.listenAddress is required when api is enabled")
	}
	return nil
}

func (c Config) ToPrettyJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

func normalizeSuffixes(suffixes []string) []string {
	out := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		s = strings.ToLower(strings.Trim(strings.TrimSpace(s), "."))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func normalizeNameservers(servers []string) []string {
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, ":") {
			s += ":53"
		}
		out = append(out, s)
	}
	return out
}

func normalizeNTPServer(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}
	if !strings.Contains(host, ":") {
		return host + ":123"
	}
	return host
}
