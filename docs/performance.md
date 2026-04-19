# Performance & Scalability

`ntp-server` is designed for high concurrency, but reaching sustained million-device scale requires the right deployment pattern.

## Practical Capacity Model

- Single instance: highly dependent on NIC, kernel tuning, and packet rate profile.
- Regional edge pool: recommended for 1M+ devices.
- Anycast or GeoDNS: recommended for global device fleets.

## Production Tuning Checklist

1. Run on Linux with tuned UDP network stack.
2. Increase socket buffers (`readBufferBytes` / `writeBufferBytes`).
3. Increase file descriptor limits (`LimitNOFILE` in systemd).
4. Use multiple instances behind L4 load balancer.
5. Keep `workers` around `2x - 6x` CPU cores and benchmark.
6. Enable observability and track packet drops at kernel level.
7. Keep upstream NTP servers close (same region where possible).

## Horizontal Scale Pattern

1. Deploy at least 3 nodes per region.
2. Route with Anycast or fast health-checked L4.
3. Keep config identical using GitOps.
4. Use rolling restarts with overlap.

## Reliability Guardrails

- Keep `serveUnsynced=true` for resilience only if your business can tolerate temporary lower trust.
- For strict environments set `serveUnsynced=false`.
- Use reverse DNS and CIDR policies to narrow inbound population when needed.
