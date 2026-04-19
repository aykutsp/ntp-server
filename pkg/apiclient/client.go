package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type HealthResponse struct {
	OK     bool      `json:"ok"`
	Synced bool      `json:"synced"`
	Time   time.Time `json:"time"`
}

type StatusResponse struct {
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

type StatsResponse struct {
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

func New(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var out HealthResponse
	err := c.getJSON(ctx, "/healthz", &out)
	return out, err
}

func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var out StatusResponse
	err := c.getJSON(ctx, "/v1/status", &out)
	return out, err
}

func (c *Client) Stats(ctx context.Context) (StatsResponse, error) {
	var out StatsResponse
	err := c.getJSON(ctx, "/v1/stats", &out)
	return out, err
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("request %s failed with %s", path, resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
