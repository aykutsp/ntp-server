package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aykutsp/ntp-server/pkg/apiclient"
)

func main() {
	var (
		baseURL = flag.String("base-url", "http://127.0.0.1:8080", "Management API base URL")
		token   = flag.String("token", "", "Bearer token")
	)
	flag.Parse()

	client := apiclient.New(*baseURL, *token, 3*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		log.Fatalf("health request failed: %v", err)
	}
	status, err := client.Status(ctx)
	if err != nil {
		log.Fatalf("status request failed: %v", err)
	}
	stats, err := client.Stats(ctx)
	if err != nil {
		log.Fatalf("stats request failed: %v", err)
	}

	fmt.Printf("Health: ok=%v synced=%v time=%s\n", health.OK, health.Synced, health.Time.Format(time.RFC3339))
	fmt.Printf("Status: stratum=%d upstream=%s offset=%.3fms\n", status.Stratum, status.Upstream, status.OffsetMillis)
	fmt.Printf("Stats: requests=%d responses=%d rateDenied=%d\n", stats.RequestsTotal, stats.ResponsesTotal, stats.RateDeniedTotal)
}
