#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "═══ Creating shared network ═══"
podman network create cdc-network 2>/dev/null || echo "Network cdc-network already exists"

echo ""
echo "═══ 1. Starting postgres1 (source OME database) ═══"
cd "$DIR/1-reader"
py -m podman_compose up -d
echo "Waiting for postgres1..."
until podman exec postgres1 pg_isready -U postgres -d omedb 2>/dev/null; do sleep 2; done
echo "postgres1 ready."

echo ""
echo "═══ 2. Starting Kafka + Debezium (CDC pipeline) ═══"
cd "$DIR/2-kafka-debezium"
py -m podman_compose up -d
echo "Waiting for Debezium (this takes ~60s)..."
until curl -sf http://localhost:8083/connectors > /dev/null 2>&1; do sleep 3; done
echo "Debezium ready."

echo ""
echo "═══ 3. Starting postgres2 + writer (Go consumer) ═══"
cd "$DIR/3-writer"
py -m podman_compose up --build -d

echo ""
echo "══════════════════════════════════════════"
echo "  ALL STARTED"
echo "══════════════════════════════════════════"
echo "  Watch writer:     podman logs -f writer"
echo "  Verify:           ./scripts/verify.sh"
echo "  Stop:             ./scripts/stop-all.sh"
echo "══════════════════════════════════════════"
