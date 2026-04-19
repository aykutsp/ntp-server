package ntp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/aykutsp/ntp-server/internal/config"
	"github.com/aykutsp/ntp-server/internal/metrics"
	"github.com/aykutsp/ntp-server/internal/policy"
	"github.com/aykutsp/ntp-server/internal/ratelimit"
)

type Status struct {
	NowUTC          time.Time `json:"nowUTC"`
	ListenAddress   string    `json:"listenAddress"`
	Workers         int       `json:"workers"`
	Synced          bool      `json:"synced"`
	Stratum         uint8     `json:"stratum"`
	OffsetMillis    float64   `json:"offsetMillis"`
	ReferenceID     string    `json:"referenceID"`
	ReferenceTime   time.Time `json:"referenceTime"`
	Upstream        string    `json:"upstream"`
	LastSyncError   string    `json:"lastSyncError"`
	ClientRateLimit float64   `json:"clientRateLimitPerSecond"`
	GlobalRateLimit float64   `json:"globalRateLimitPerSecond"`
}

type Server struct {
	cfg     config.Config
	logger  *slog.Logger
	metrics *metrics.Counters

	conn          *net.UDPConn
	aclPolicy     *policy.CIDRPolicy
	dnsPolicy     *policy.DNSPolicy
	clientLimiter *ratelimit.KeyedLimiter
	globalLimiter *ratelimit.TokenBucket
	syncState     *syncState
	defaults      ResponseDefaults

	readTimeout  time.Duration
	writeTimeout time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewServer(cfg config.Config, logger *slog.Logger, m *metrics.Counters) (*Server, error) {
	addr, err := net.ResolveUDPAddr("udp", cfg.NTP.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("resolve listen address %q: %w", cfg.NTP.ListenAddress, err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen udp %q: %w", cfg.NTP.ListenAddress, err)
	}
	if cfg.NTP.ReadBufferBytes > 0 {
		_ = conn.SetReadBuffer(cfg.NTP.ReadBufferBytes)
	}
	if cfg.NTP.WriteBufferBytes > 0 {
		_ = conn.SetWriteBuffer(cfg.NTP.WriteBufferBytes)
	}

	acl, err := policy.NewCIDRPolicy(cfg.Policy.AllowCIDRs, cfg.Policy.DenyCIDRs)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	dnsPolicy := policy.NewDNSPolicy(policy.DNSPolicyConfig{
		Enabled:            cfg.Policy.DNS.EnableReverseLookup,
		AllowSuffixes:      cfg.Policy.DNS.AllowSuffixes,
		DenySuffixes:       cfg.Policy.DNS.DenySuffixes,
		CacheTTL:           time.Duration(cfg.Policy.DNS.CacheTTLSeconds) * time.Second,
		LookupTimeout:      time.Duration(cfg.Policy.DNS.LookupTimeoutMillis) * time.Millisecond,
		AllowOnLookupError: cfg.Policy.DNS.AllowOnLookupError,
		Nameservers:        cfg.Policy.DNS.ResolverNameservers,
	})

	defaultRef := ReferenceID(cfg.NTP.ReferenceID)
	s := &Server{
		cfg:     cfg,
		logger:  logger,
		metrics: m,

		conn:      conn,
		aclPolicy: acl,
		dnsPolicy: dnsPolicy,
		clientLimiter: ratelimit.NewKeyedLimiter(
			cfg.NTP.ClientRateLimitPerSecond,
			cfg.NTP.ClientRateLimitBurst,
			10*time.Minute,
			128,
		),
		globalLimiter: ratelimit.NewTokenBucket(cfg.NTP.GlobalRateLimitPerSecond, cfg.NTP.GlobalRateLimitBurst),
		syncState:     newSyncState(defaultRef),
		defaults: ResponseDefaults{
			ServeUnsynced:    cfg.NTP.ServeUnsynced,
			UnsyncedStratum:  cfg.NTP.UnsyncedStratum,
			LocalReferenceID: defaultRef,
			PrecisionExp:     cfg.NTP.PrecisionExponent,
			RootDelay:        time.Duration(cfg.NTP.RootDelayMillis * float64(time.Millisecond)),
			RootDispersion:   time.Duration(cfg.NTP.RootDispersionMillis * float64(time.Millisecond)),
		},
		readTimeout:  time.Duration(cfg.NTP.ReadTimeoutMillis) * time.Millisecond,
		writeTimeout: time.Duration(cfg.NTP.WriteTimeoutMillis) * time.Millisecond,
	}
	return s, nil
}

func (s *Server) Start(parent context.Context) {
	s.ctx, s.cancel = context.WithCancel(parent)

	if s.cfg.Upstream.Enabled {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			runUpstreamSyncLoop(s.ctx, s.cfg.Upstream, s.syncState, s.metrics, s.logger)
		}()
	}

	for i := 0; i < s.cfg.NTP.Workers; i++ {
		s.wg.Add(1)
		go func(id int) {
			defer s.wg.Done()
			s.workerLoop(id)
		}(i + 1)
	}

	s.logger.Info("NTP server started",
		"listen", s.cfg.NTP.ListenAddress,
		"workers", s.cfg.NTP.Workers,
		"upstream_enabled", s.cfg.Upstream.Enabled,
	)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) workerLoop(workerID int) {
	buf := make([]byte, 512)

	for {
		if s.ctx.Err() != nil {
			return
		}
		if s.readTimeout > 0 {
			_ = s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
		}

		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			if s.ctx.Err() != nil {
				return
			}
			s.logger.Debug("udp read error", "worker", workerID, "error", err.Error())
			continue
		}

		s.metrics.IncRequests()
		s.metrics.AddBytesIn(n)

		if n < packetLen || !IsClientRequest(buf[:n]) {
			s.metrics.IncMalformed()
			continue
		}

		request := make([]byte, packetLen)
		copy(request, buf[:packetLen])
		receivedAt := time.Now().UTC()

		if !s.aclPolicy.Allow(addr.IP) {
			s.metrics.IncACLDenied()
			s.sendKissOfDeath(addr, request, receivedAt)
			continue
		}

		if ok, reason := s.dnsPolicy.Allow(s.ctx, addr.IP); !ok {
			s.metrics.IncDNSDenied()
			s.logger.Debug("request denied by reverse DNS policy",
				"client", addr.IP.String(),
				"reason", reason,
			)
			s.sendKissOfDeath(addr, request, receivedAt)
			continue
		}

		if !s.globalLimiter.Allow(receivedAt) || !s.clientLimiter.Allow(addr.IP.String(), receivedAt) {
			s.metrics.IncRateDenied()
			s.sendKissOfDeath(addr, request, receivedAt)
			continue
		}

		snapshot := s.syncState.Snapshot()
		if !snapshot.Synced && !s.cfg.NTP.ServeUnsynced {
			continue
		}

		clockOffset := snapshot.Offset
		response := BuildResponse(
			request,
			receivedAt.Add(clockOffset),
			time.Now().UTC().Add(clockOffset),
			snapshot,
			s.defaults,
		)

		if s.writeTimeout > 0 {
			_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
		}

		written, err := s.conn.WriteToUDP(response[:], addr)
		if err != nil {
			s.logger.Debug("udp write error", "client", addr.String(), "error", err.Error())
			continue
		}
		s.metrics.IncResponses()
		s.metrics.AddBytesOut(written)
	}
}

func (s *Server) sendKissOfDeath(addr *net.UDPAddr, req []byte, now time.Time) {
	if !s.cfg.NTP.EnableKissOfDeathOnDeny || len(req) < packetLen {
		return
	}

	code := strings.TrimSpace(s.cfg.NTP.KissOfDeathResponseOnDeny)
	if code == "" {
		code = "RATE"
	}
	kod := BuildKissOfDeath(req, now, now, code)

	if s.writeTimeout > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	if written, err := s.conn.WriteToUDP(kod[:], addr); err == nil {
		s.metrics.IncKissOfDeath()
		s.metrics.AddBytesOut(written)
	}
}

func (s *Server) Status() Status {
	ss := s.syncState.Snapshot()
	return Status{
		NowUTC:          time.Now().UTC().Add(ss.Offset),
		ListenAddress:   s.cfg.NTP.ListenAddress,
		Workers:         s.cfg.NTP.Workers,
		Synced:          ss.Synced,
		Stratum:         s.effectiveStratum(ss),
		OffsetMillis:    durationMillis(ss.Offset),
		ReferenceID:     strings.TrimSpace(string(ss.ReferenceID[:])),
		ReferenceTime:   ss.ReferenceTime,
		Upstream:        ss.Upstream,
		LastSyncError:   ss.LastError,
		ClientRateLimit: s.cfg.NTP.ClientRateLimitPerSecond,
		GlobalRateLimit: s.cfg.NTP.GlobalRateLimitPerSecond,
	}
}

func (s *Server) effectiveStratum(ss SyncSnapshot) uint8 {
	if ss.Synced {
		if ss.Stratum == 0 {
			return 1
		}
		return ss.Stratum
	}
	return s.cfg.NTP.UnsyncedStratum
}

func (s *Server) Config() config.Config {
	return s.cfg
}
