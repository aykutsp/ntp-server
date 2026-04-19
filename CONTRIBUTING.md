# Contributing

## Development

1. Create a branch from `main`.
2. Run `go test ./...`.
3. Run `gofmt -w .`.
4. Open a pull request with clear context and validation notes.

## Commit Quality

- Keep commits focused.
- Include tests for behavioral changes.
- Avoid unrelated formatting churn.

## Code Review Expectations

- Reliability and protocol correctness first.
- Backward compatibility for config fields.
- Operational safety (timeouts, limits, shutdown behavior).
