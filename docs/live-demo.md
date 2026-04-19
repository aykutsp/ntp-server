# Live Demo on GitHub

This repository includes `.github/workflows/live-demo.yml` which:

1. Builds the Docker image.
2. Starts the container with UDP + HTTP ports.
3. Verifies `healthz` and `status` endpoints.
4. Executes `cmd/ntp-query` against the live server.

## How to Run

1. Push to `main` or `master`, or trigger the workflow manually from GitHub Actions.
2. Open the workflow logs.
3. Inspect health JSON, status payload, and NTP check output.

This provides a practical, reproducible live demonstration for stakeholders.
