# NTP Server

[![CI](https://github.com/aykutsp/ntp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/aykutsp/ntp-server/actions/workflows/ci.yml)
[![Live Demo](https://github.com/aykutsp/ntp-server/actions/workflows/live-demo.yml/badge.svg)](https://github.com/aykutsp/ntp-server/actions/workflows/live-demo.yml)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE)

A production-grade NTP server written in Go. Runs on Linux, macOS, and Windows. Designed for high-throughput environments with full RFC 4330 compliance, upstream synchronization, access control, rate limiting, and an HTTP management API.

---

## Live Demo

**[https://aykutsp.github.io/ntp-server](https://aykutsp.github.io/ntp-server)**

The demo page shows:
- Live UTC clock ticking in your browser
- Stratum, Reference ID, offset and RTT from the last CI run
- Upstream sync status
- Copy-paste connection details and client examples for chrony, ntpd, macOS, Windows, and Python

The page is backed by a `docs/demo-result.json` file that gets written on every CI run. The workflow:
1. Builds the Docker image and starts the container
2. Hits `/healthz` and `/v1/status`
3. Runs `cmd/ntp-query` against the live container
4. Writes the NTP + status JSON into `docs/demo-result.json` and commits it
5. GitHub Pages picks up the change and redeploys automatically

[View CI workflow](https://github.com/aykutsp/ntp-server/actions/workflows/live-demo.yml) · [Trigger manually](https://github.com/aykutsp/ntp-server/actions/workflows/live-demo.yml)

---

## Quick Start

```bash
git clone https://github.com/aykutsp/ntp-server.git
cd ntp-server
go run ./cmd/ntp-server -config ./configs/server.example.json
```

In a second terminal:

```bash
# Health check
curl http://127.0.0.1:8080/healthz

# Server status
curl http://127.0.0.1:8080/v1/status

# NTP query
go run ./cmd/ntp-query -server 127.0.0.1:12300
```

---

## Testing with Your NTP Client

You can point any standard NTP client at this server. Below are the most common methods.

### Linux — chrony

Edit `/etc/chrony.conf`:

```
server 127.0.0.1 port 12300 iburst
```

Then restart:

```bash
sudo systemctl restart chronyd
chronyc tracking
chronyc sources -v
```

### Linux — ntpd (ntp package)

Edit `/etc/ntp.conf`:

```
server 127.0.0.1 port 12300 iburst
```

Restart and verify:

```bash
sudo systemctl restart ntp
ntpq -p
```

### macOS — sntp (built-in)

```bash
sntp -S 127.0.0.1
```

Or with a custom port using `ntpdate`:

```bash
sudo ntpdate -u -p 4 127.0.0.1
```

### Windows — w32tm

```powershell
w32tm /config /manualpeerlist:"127.0.0.1" /syncfromflags:manual /reliable:YES /update
w32tm /resync /force
w32tm /query /status
```

### Python — ntplib

```python
import ntplib, datetime

c = ntplib.NTPClient()
# Change host/port to your server address
response = c.request('127.0.0.1', port=12300, version=4)
print("Offset :", response.offset)
print("Stratum:", response.stratum)
print("Time   :", datetime.datetime.utcfromtimestamp(response.tx_time))
```

Install with: `pip install ntplib`

### Built-in query tool

```bash
go run ./cmd/ntp-query -server 127.0.0.1:12300
```

---

## How NTP Works

NTP (Network Time Protocol) synchronizes clocks across a network using a hierarchical model called the **stratum system**.

```
Stratum 0  ──  Atomic clocks, GPS receivers (reference clocks)
    │
Stratum 1  ──  Servers directly connected to Stratum 0 (e.g. time.google.com)
    │
Stratum 2  ──  Servers synced from Stratum 1  ◄── this server operates here
    │
Stratum 3+ ──  Your devices, VMs, containers
```

### The 4-Timestamp Exchange (RFC 4330)

Every NTP transaction uses exactly four timestamps to calculate clock offset and round-trip delay:

```
Client                          Server
  │                               │
  │── Request ──────────────────► │  T1 = client transmit time
  │                               │  T2 = server receive time
  │◄─────────────────── Response ─│  T3 = server transmit time
  │                               │
T4 = client receive time

Offset = ((T2 - T1) + (T3 - T4)) / 2
Delay  = (T4 - T1) - (T3 - T2)
```

The offset tells the client how far its clock is from the server. The delay accounts for asymmetric network paths.

### What This Server Does

1. **Listens on UDP 123 (or any configured port)** — NTP uses UDP because it is stateless and low-latency. TCP handshakes would add unacceptable jitter to time measurements.

2. **Upstream synchronization** — On startup and every N seconds, the server queries multiple upstream NTP servers (Cloudflare, Google, pool.ntp.org by default). It collects samples, picks the lowest-latency candidates, and takes the median offset to reduce noise. This keeps the server's own clock accurate.

3. **Packet processing** — Each incoming 48-byte UDP packet is validated (mode=3, version 3 or 4). The server fills in the four NTP timestamps and returns a 48-byte response.

4. **Access control** — CIDR allow/deny lists and optional reverse-DNS policy filter which clients can receive responses.

5. **Rate limiting** — A per-client token bucket and a global token bucket prevent abuse. Denied clients receive a Kiss-of-Death packet (stratum 0, reference ID = `RATE`) so well-behaved NTP clients back off automatically.

6. **Kiss-of-Death (KoD)** — A standard NTP mechanism. When a client is denied, the server sends a special response with stratum=0 and a 4-byte ASCII code in the reference ID field. RFC-compliant clients stop polling when they receive this.

---

## Architecture for 1 Million Concurrent Clients

Serving 1M+ devices simultaneously requires a deliberate deployment architecture. A single process can handle a very high packet rate, but geographic distribution and redundancy are essential at this scale.

### Capacity Model

A single instance on a modern Linux server (8–16 cores, tuned UDP stack) can sustain roughly **50,000–200,000 requests/second** depending on NIC throughput, kernel buffer configuration, and packet size. NTP packets are tiny (48 bytes), so the bottleneck is almost always the kernel UDP receive path, not CPU.

To reach 1M concurrent devices (assuming each polls every 64–1024 seconds), the actual packet rate is modest — roughly **1,000–15,000 packets/second** — well within a single instance. The challenge is **reliability, latency, and geographic distribution**, not raw throughput.

### Recommended Deployment Pattern

```
                        ┌─────────────────────────────────┐
                        │         Anycast / GeoDNS         │
                        │   pool.yourdomain.com  UDP 123   │
                        └────────────┬────────────────────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              │                      │                       │
     ┌────────▼──────┐     ┌─────────▼─────┐     ┌──────────▼────┐
     │  Region: US   │     │  Region: EU   │     │  Region: APAC │
     │               │     │               │     │               │
     │  node-1 :123  │     │  node-1 :123  │     │  node-1 :123  │
     │  node-2 :123  │     │  node-2 :123  │     │  node-2 :123  │
     │  node-3 :123  │     │  node-3 :123  │     │  node-3 :123  │
     └───────────────┘     └───────────────┘     └───────────────┘
              │                      │                       │
     ┌────────▼──────────────────────▼───────────────────────▼────┐
     │              Upstream NTP (Stratum 1)                       │
     │   time.cloudflare.com  time.google.com  pool.ntp.org        │
     └─────────────────────────────────────────────────────────────┘
```

### Layer-by-Layer Breakdown

**1. Anycast or GeoDNS routing**

Anycast assigns the same IP to multiple nodes in different regions. The network automatically routes each client to the nearest node. GeoDNS achieves similar results at the DNS layer. Both approaches reduce latency and provide transparent failover — if a node goes down, traffic shifts to the next closest node without any client-side change.

**2. Regional node clusters (minimum 3 per region)**

Running at least 3 nodes per region provides:
- Redundancy if one node loses upstream sync or crashes
- Horizontal capacity for traffic spikes
- Rolling restarts without downtime

All nodes in a region share identical configuration (managed via GitOps). Each node independently syncs from upstream NTP servers.

**3. L4 load balancer (UDP)**

Within a region, an L4 load balancer (e.g. HAProxy, AWS NLB, or Linux IPVS) distributes UDP packets across nodes. Because NTP is stateless (each request is independent), no session affinity is needed. Consistent hashing on source IP is optional but reduces per-client jitter.

**4. Worker pool per node**

Each node runs a configurable number of goroutine workers sharing a single UDP socket. Workers call `ReadFromUDP` concurrently, process the packet, and call `WriteToUDP`. The kernel handles the actual socket multiplexing. Recommended worker count: `2x–6x` CPU cores, tuned by benchmarking.

**5. Kernel-level UDP tuning**

At 1M+ device scale, the kernel UDP receive buffer is the most common bottleneck:

```bash
# Increase socket buffer limits
sysctl -w net.core.rmem_max=134217728
sysctl -w net.core.wmem_max=134217728
sysctl -w net.core.netdev_max_backlog=65536

# Increase file descriptor limits (systemd)
# LimitNOFILE=1048576 in the service unit
```

The server's `readBufferBytes` and `writeBufferBytes` config fields map directly to `SO_RCVBUF` / `SO_SNDBUF` on the socket.

**6. Rate limiting and abuse protection**

At internet scale, a public NTP server will receive amplification attack traffic. The server's two-tier rate limiter handles this:
- **Global bucket** — caps total packet rate across all clients
- **Per-client bucket** — caps individual client polling rate
- **Kiss-of-Death responses** — signals RFC-compliant clients to back off

**7. Observability**

Each node exposes `/v1/stats` with counters for requests, responses, rate-denied, ACL-denied, sync success/failure, and bytes in/out. Feed these into Prometheus + Grafana or any metrics pipeline to detect packet drops, upstream sync failures, or traffic anomalies in real time.

### Configuration for High Scale

```json
{
  "ntp": {
    "workers": 32,
    "readBufferBytes": 134217728,
    "writeBufferBytes": 134217728,
    "globalRateLimitPerSecond": 500000,
    "globalRateLimitBurst": 120000,
    "clientRateLimitPerSecond": 64,
    "clientRateLimitBurst": 128
  }
}
```

---

## Docker

```bash
docker compose up --build -d
```

Ports:
- UDP `12300` — NTP
- TCP `8080` — HTTP management API

---

## Configuration

Reference file: [`configs/server.example.json`](./configs/server.example.json)

| Field | Description |
|---|---|
| `ntp.listenAddress` | UDP bind address and port |
| `ntp.workers` | Number of concurrent worker goroutines |
| `ntp.readBufferBytes` / `writeBufferBytes` | Kernel socket buffer sizes |
| `ntp.clientRateLimitPerSecond` | Per-client request rate cap |
| `ntp.globalRateLimitPerSecond` | Global request rate cap |
| `policy.allowCIDRs` / `denyCIDRs` | IP access control |
| `policy.dns.*` | Reverse DNS policy |
| `upstream.servers` | Upstream NTP servers to sync from |
| `api.listenAddress` | HTTP management API bind address |
| `api.authToken` | Optional Bearer token for API auth |

---

## Running as a Service

### Linux (systemd)

```bash
sudo systemctl daemon-reload
sudo systemctl enable ntp-server
sudo systemctl start ntp-server
```

### macOS (launchd)

```bash
sudo cp deploy/launchd/com.ntp.server.plist /Library/LaunchDaemons/
sudo launchctl load /Library/LaunchDaemons/com.ntp.server.plist
```

### Windows (Service)

```powershell
.\deploy\windows\install-service.ps1 `
  -BinaryPath "C:\Program Files\NtpServer\ntp-server.exe" `
  -ConfigPath "C:\ProgramData\NtpServer\config.json"
```

---

## API Client Library

Package: [`pkg/apiclient`](./pkg/apiclient/client.go)

```go
client := apiclient.New("http://127.0.0.1:8080", "", 2*time.Second)
status, err := client.Status(ctx)
fmt.Println(status.Synced, status.OffsetMillis)
```

---

## Development

```bash
go test ./... -race -count=1
go run ./cmd/ntp-server -print-default-config
```

Multi-platform build:

```bash
./scripts/build.sh v1.0.0      # Linux / macOS
.\scripts\build.ps1 -Version v1.0.0  # Windows
```

---

## License

MIT — see [LICENSE](./LICENSE).
