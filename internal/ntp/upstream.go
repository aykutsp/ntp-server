package ntp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aykutsp/ntp-server/internal/config"
	"github.com/aykutsp/ntp-server/internal/metrics"
)

type syncState struct {
	mu       sync.RWMutex
	snapshot SyncSnapshot
}

type upstreamSample struct {
	server         string
	stratum        uint8
	referenceID    [4]byte
	referenceTime  time.Time
	offset         time.Duration
	delay          time.Duration
	rootDelay      time.Duration
	rootDispersion time.Duration
}

func newSyncState(localRef [4]byte) *syncState {
	return &syncState{
		snapshot: SyncSnapshot{
			Synced:         false,
			Stratum:        16,
			ReferenceID:    localRef,
			ReferenceTime:  time.Now().UTC(),
			Offset:         0,
			RootDelay:      1 * time.Millisecond,
			RootDispersion: 1 * time.Millisecond,
			Upstream:       "",
			LastError:      "not synced yet",
		},
	}
}

func (s *syncState) Snapshot() SyncSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *syncState) Update(sample upstreamSample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshot.Synced = true
	s.snapshot.Stratum = sample.stratum
	if s.snapshot.Stratum == 0 {
		s.snapshot.Stratum = 1
	}
	s.snapshot.ReferenceID = sample.referenceID
	s.snapshot.ReferenceTime = sample.referenceTime
	s.snapshot.Offset = sample.offset
	s.snapshot.RootDelay = sample.rootDelay
	s.snapshot.RootDispersion = sample.rootDispersion
	s.snapshot.Upstream = sample.server
	s.snapshot.LastError = ""
}

func (s *syncState) MarkError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.snapshot.LastError = err.Error()
	}
}

func runUpstreamSyncLoop(ctx context.Context, cfg config.UpstreamConfig, state *syncState, m *metrics.Counters, logger *slog.Logger) {
	if !cfg.Enabled {
		return
	}
	interval := time.Duration(cfg.SyncIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	// Run an immediate first sync to reduce warmup.
	syncRound(cfg, state, m, logger)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncRound(cfg, state, m, logger)
		}
	}
}

func syncRound(cfg config.UpstreamConfig, state *syncState, m *metrics.Counters, logger *slog.Logger) {
	sample, err := bestUpstreamSample(cfg)
	if err != nil {
		state.MarkError(err)
		m.IncSyncFailure()
		logger.Warn("upstream sync failed", "error", err.Error())
		return
	}
	state.Update(sample)
	m.IncSyncSuccess()
	logger.Debug("upstream sync success",
		"server", sample.server,
		"stratum", sample.stratum,
		"offset_ms", durationMillis(sample.offset),
		"delay_ms", durationMillis(sample.delay),
	)
}

func bestUpstreamSample(cfg config.UpstreamConfig) (upstreamSample, error) {
	var samples []upstreamSample
	var errs []error

	timeout := time.Duration(cfg.TimeoutMillis) * time.Millisecond
	maxOffset := time.Duration(cfg.MaxAcceptedOffsetMillis) * time.Millisecond

	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}
	if maxOffset <= 0 {
		maxOffset = 1500 * time.Millisecond
	}

	for _, server := range cfg.Servers {
		for i := 0; i < cfg.SamplesPerSync; i++ {
			s, err := queryUpstream(server, timeout)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", server, err))
				continue
			}
			if absDuration(s.offset) > maxOffset {
				errs = append(errs, fmt.Errorf("%s: offset %s exceeds max accepted %s", server, s.offset, maxOffset))
				continue
			}
			samples = append(samples, s)
		}
	}

	if len(samples) == 0 {
		return upstreamSample{}, errors.Join(errs...)
	}

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].delay < samples[j].delay
	})

	topN := len(samples)
	if topN > 5 {
		topN = 5
	}

	offsets := make([]time.Duration, 0, topN)
	for i := 0; i < topN; i++ {
		offsets = append(offsets, samples[i].offset)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })

	best := samples[0]
	best.offset = offsets[len(offsets)/2] // median offset of low-latency samples
	best.referenceTime = time.Now().UTC().Add(best.offset)
	return best, nil
}

func queryUpstream(server string, timeout time.Duration) (upstreamSample, error) {
	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return upstreamSample{}, fmt.Errorf("resolve: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return upstreamSample{}, fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	var req [packetLen]byte
	req[0] = (0 << 6) | (4 << 3) | 3
	req[2] = 4
	req[3] = 0xEC

	t1 := time.Now().UTC()
	binary.BigEndian.PutUint64(req[40:48], ToTimestamp(t1))

	if _, err := conn.Write(req[:]); err != nil {
		return upstreamSample{}, fmt.Errorf("write: %w", err)
	}

	var resp [256]byte
	n, err := conn.Read(resp[:])
	if err != nil {
		return upstreamSample{}, fmt.Errorf("read: %w", err)
	}
	if n < packetLen {
		return upstreamSample{}, fmt.Errorf("short response: %d bytes", n)
	}
	mode := resp[0] & 0x7
	if mode != 4 {
		return upstreamSample{}, fmt.Errorf("unexpected mode %d", mode)
	}

	stratum := resp[1]
	if stratum == 0 {
		code := string(resp[12:16])
		return upstreamSample{}, fmt.Errorf("kiss-of-death from upstream (%s)", stringsTrim(code))
	}

	t4 := time.Now().UTC()
	t2 := ReadTimestamp(resp[:], 32)
	t3 := ReadTimestamp(resp[:], 40)
	if t2.IsZero() || t3.IsZero() {
		return upstreamSample{}, errors.New("invalid upstream timestamps")
	}

	offset := ((t2.Sub(t1)) + (t3.Sub(t4))) / 2
	delay := (t4.Sub(t1)) - (t3.Sub(t2))
	if delay < 0 {
		delay = 0
	}

	var refID [4]byte
	copy(refID[:], resp[12:16])

	return upstreamSample{
		server:         server,
		stratum:        stratum,
		referenceID:    refID,
		referenceTime:  ReadTimestamp(resp[:], 16),
		offset:         offset,
		delay:          delay,
		rootDelay:      shortToDuration(binary.BigEndian.Uint32(resp[4:8])),
		rootDispersion: shortToDuration(binary.BigEndian.Uint32(resp[8:12])),
	}, nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func durationMillis(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func stringsTrim(code string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return -1
		}
		return r
	}, code)
}
