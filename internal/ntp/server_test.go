package ntp

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/aykutsp/ntp-server/internal/config"
	"github.com/aykutsp/ntp-server/internal/metrics"
)

func TestServerRespondsToNTPRequest(t *testing.T) {
	cfg := config.Default()
	cfg.NTP.ListenAddress = "127.0.0.1:0"
	cfg.NTP.Workers = 1
	cfg.Upstream.Enabled = false
	cfg.Policy.DNS.EnableReverseLookup = false
	cfg.NTP.ClientRateLimitPerSecond = 0
	cfg.NTP.GlobalRateLimitPerSecond = 0

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.NewCounters()

	srv, err := NewServer(cfg, logger, m)
	if err != nil {
		t.Fatalf("server setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Start(ctx)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	conn, err := net.Dial("udp", srv.conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	var req [48]byte
	req[0] = (0 << 6) | (4 << 3) | 3
	copy(req[40:48], u64Bytes(ToTimestamp(time.Now().UTC())))

	if _, err := conn.Write(req[:]); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp [128]byte
	n, err := conn.Read(resp[:])
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n < 48 {
		t.Fatalf("short response: %d", n)
	}
	if mode := resp[0] & 0x7; mode != 4 {
		t.Fatalf("expected mode=4, got=%d", mode)
	}
}
