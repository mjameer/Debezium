FROM docker.io/golang:1.23-bookworm AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download 2>/dev/null; true
COPY *.go ./
RUN go mod tidy && CGO_ENABLED=0 go build -o /writer .

# Use postgres:17 as base so pg_dump/pg_restore match the server version
FROM docker.io/postgres:17-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /writer /usr/local/bin/writer
CMD ["writer"]
