package metrics

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type Counters struct {
	startedAt time.Time

	requestsTotal    atomic.Uint64
	responsesTotal   atomic.Uint64
	malformedTotal   atomic.Uint64
	aclDeniedTotal   atomic.Uint64
	dnsDeniedTotal   atomic.Uint64
	rateDeniedTotal  atomic.Uint64
	kissOfDeathTotal atomic.Uint64
	bytesInTotal     atomic.Uint64
	bytesOutTotal    atomic.Uint64
	syncSuccessTotal atomic.Uint64
	syncFailureTotal atomic.Uint64
}

type Snapshot struct {
	StartedAt        time.Time `json:"startedAt"`
	UptimeSeconds    float64   `json:"uptimeSeconds"`
	RequestsTotal    uint64    `json:"requestsTotal"`
	ResponsesTotal   uint64    `json:"responsesTotal"`
	MalformedTotal   uint64    `json:"malformedTotal"`
	ACLDeniedTotal   uint64    `json:"aclDeniedTotal"`
	DNSDeniedTotal   uint64    `json:"dnsDeniedTotal"`
	RateDeniedTotal  uint64    `json:"rateDeniedTotal"`
	KissOfDeathTotal uint64    `json:"kissOfDeathTotal"`
	BytesInTotal     uint64    `json:"bytesInTotal"`
	BytesOutTotal    uint64    `json:"bytesOutTotal"`
	SyncSuccessTotal uint64    `json:"syncSuccessTotal"`
	SyncFailureTotal uint64    `json:"syncFailureTotal"`
}

func NewCounters() *Counters {
	return &Counters{startedAt: time.Now().UTC()}
}

func (c *Counters) IncRequests()      { c.requestsTotal.Add(1) }
func (c *Counters) IncResponses()     { c.responsesTotal.Add(1) }
func (c *Counters) IncMalformed()     { c.malformedTotal.Add(1) }
func (c *Counters) IncACLDenied()     { c.aclDeniedTotal.Add(1) }
func (c *Counters) IncDNSDenied()     { c.dnsDeniedTotal.Add(1) }
func (c *Counters) IncRateDenied()    { c.rateDeniedTotal.Add(1) }
func (c *Counters) IncKissOfDeath()   { c.kissOfDeathTotal.Add(1) }
func (c *Counters) AddBytesIn(n int)  { c.bytesInTotal.Add(uint64(n)) }
func (c *Counters) AddBytesOut(n int) { c.bytesOutTotal.Add(uint64(n)) }
func (c *Counters) IncSyncSuccess()   { c.syncSuccessTotal.Add(1) }
func (c *Counters) IncSyncFailure()   { c.syncFailureTotal.Add(1) }

func (c *Counters) Snapshot() Snapshot {
	uptime := time.Since(c.startedAt).Seconds()
	if uptime < 0 {
		uptime = 0
	}

	return Snapshot{
		StartedAt:        c.startedAt,
		UptimeSeconds:    uptime,
		RequestsTotal:    c.requestsTotal.Load(),
		ResponsesTotal:   c.responsesTotal.Load(),
		MalformedTotal:   c.malformedTotal.Load(),
		ACLDeniedTotal:   c.aclDeniedTotal.Load(),
		DNSDeniedTotal:   c.dnsDeniedTotal.Load(),
		RateDeniedTotal:  c.rateDeniedTotal.Load(),
		KissOfDeathTotal: c.kissOfDeathTotal.Load(),
		BytesInTotal:     c.bytesInTotal.Load(),
		BytesOutTotal:    c.bytesOutTotal.Load(),
		SyncSuccessTotal: c.syncSuccessTotal.Load(),
		SyncFailureTotal: c.syncFailureTotal.Load(),
	}
}

func (c *Counters) Prometheus() string {
	s := c.Snapshot()

	var b strings.Builder
	writeGauge := func(name string, val float64, help string) {
		b.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
		b.WriteString(fmt.Sprintf("# TYPE %s gauge\n", name))
		b.WriteString(fmt.Sprintf("%s %.6f\n", name, val))
	}
	writeCounter := func(name string, val uint64, help string) {
		b.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
		b.WriteString(fmt.Sprintf("# TYPE %s counter\n", name))
		b.WriteString(fmt.Sprintf("%s %d\n", name, val))
	}

	writeGauge("ntp_server_uptime_seconds", s.UptimeSeconds, "Process uptime in seconds.")
	writeCounter("ntp_server_requests_total", s.RequestsTotal, "Total NTP requests received.")
	writeCounter("ntp_server_responses_total", s.ResponsesTotal, "Total NTP responses sent.")
	writeCounter("ntp_server_malformed_total", s.MalformedTotal, "Malformed NTP packet count.")
	writeCounter("ntp_server_acl_denied_total", s.ACLDeniedTotal, "Requests denied by CIDR ACL.")
	writeCounter("ntp_server_dns_denied_total", s.DNSDeniedTotal, "Requests denied by reverse DNS policy.")
	writeCounter("ntp_server_rate_denied_total", s.RateDeniedTotal, "Requests denied by rate limiters.")
	writeCounter("ntp_server_kiss_of_death_total", s.KissOfDeathTotal, "Kiss-o'-Death packets sent.")
	writeCounter("ntp_server_bytes_in_total", s.BytesInTotal, "Ingress bytes.")
	writeCounter("ntp_server_bytes_out_total", s.BytesOutTotal, "Egress bytes.")
	writeCounter("ntp_server_sync_success_total", s.SyncSuccessTotal, "Upstream sync successes.")
	writeCounter("ntp_server_sync_failure_total", s.SyncFailureTotal, "Upstream sync failures.")

	return b.String()
}
