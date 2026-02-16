#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")/.." && pwd)"
echo "Stopping 3-writer..."
cd "$DIR/3-writer" && py -m podman_compose down -v 2>/dev/null || true
echo "Stopping 2-kafka-debezium..."
cd "$DIR/2-kafka-debezium" && py -m podman_compose down -v 2>/dev/null || true
echo "Stopping 1-reader..."
cd "$DIR/1-reader" && py -m podman_compose down -v 2>/dev/null || true
echo "Removing network..."
podman network rm cdc-network 2>/dev/null || true
echo "Done."
