FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod ./
COPY . ./

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ntp-server ./cmd/ntp-server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/ntp-server /usr/local/bin/ntp-server
COPY configs/server.example.json /etc/ntp-server/config.json

EXPOSE 12300/udp
EXPOSE 8080/tcp

ENTRYPOINT ["/usr/local/bin/ntp-server", "-config", "/etc/ntp-server/config.json"]
