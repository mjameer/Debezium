# wal-cdc-replication-poc

Zero-data-loss OME → Federation replication. Three isolated services.

---

## Architecture — Who Does What

```
┌──────────────────────┐      ┌──────────────────────────┐      ┌──────────────────────────┐
│  1-reader/           │      │  2-kafka-debezium/       │      │  3-writer/               │
│                      │      │                          │      │                          │
│  postgres1           │      │  Debezium                │      │  writer (Go)             │
│  (OME source DB)     │      │  READS postgres1 WAL     │      │  READS Kafka topics      │
│  13 tables           │      │  Produces Kafka events   │      │  WRITES to postgres2     │
│  ~8500 rows          │      │                          │      │                          │
│                      │ WAL  │  Kafka                   │ CDC  │  postgres2               │
│  Has the data.       │─────▶│  Holds CDC events        │─────▶│  (Federation target DB)  │
│  Produces WAL on     │      │  in topics per table     │      │  Starts EMPTY            │
│  every change.       │      │                          │      │  Receives all data       │
│                      │      │  Redpanda Console        │      │                          │
│  READS: nothing      │      │  UI at localhost:8080    │      │  Also does:              │
│  WRITES: nothing     │      │                          │      │  - create replication    │
│  (it IS the source)  │      │  READS: postgres1 WAL    │      │    slot on postgres1     │
│                      │      │  WRITES: Kafka topics    │      │  - pg_dump → pg_restore  │
│                      │      │  Does NOT touch DBs      │      │  - deploy connector      │
└──────────────────────┘      └──────────────────────────┘      └──────────────────────────┘
```

Each folder has its own `docker-compose.yml`. They share a Docker network called `cdc-network`.

---

## Quick Start

```powershell
# Start everything in order
.\scripts\start-all.ps1

# Watch the writer do its thing
podman logs -f writer

# Verify replication
.\scripts\verify.ps1

# Stop everything
.\scripts\stop-all.ps1
```

## Or Start Each Manually

```powershell
# Create shared network first
podman network create cdc-network

# 1. Source database
cd 1-reader
py -m podman_compose up -d
cd ..

# 2. Kafka + Debezium pipeline
cd 2-kafka-debezium
py -m podman_compose up -d
cd ..
# Wait for Debezium: curl http://localhost:8083/connectors

# 3. Target database + Go writer
cd 3-writer
py -m podman_compose up --build -d
cd ..
# Watch: podman logs -f writer
```

---

## What the Writer Does (the interesting part)

The Go service in `3-writer/` runs this sequence:

```
STEP 1:  Wait for postgres1, postgres2, Kafka, Debezium

STEP 2:  CREATE REPLICATION SLOT on postgres1
         This bookmarks the WAL. PostgreSQL keeps everything from here.

STEP 3:  pg_dump postgres1 → file
         MVCC snapshot: sees all data committed up to now.

STEP 4:  pg_restore file → postgres2
         postgres2 now has the bulk data.

STEP 5:  Deploy Debezium connector (snapshot.mode=never)
         Debezium starts reading WAL from the slot position.

STEP 6:  Start 13 Kafka consumers (one per table)
         Each consumer reads CDC events and upserts into postgres2.

STEP 7:  Insert test data into postgres1
         Proves that new writes stream through CDC to postgres2.

STEP 8:  Verify test data in postgres2 ✅
```

---

## How Zero Data Loss Works

```
Timeline ────────────────────────────────────────────────▶

  T1 (slot created)         T2 (dump starts)
  │                         │
  ▼                         ▼

  Row written BEFORE T1     →  in dump YES, in WAL NO   → dump covers it
  Row written T1 to T2      →  in dump YES, in WAL YES  → both (upsert dedupes)
  Row written AFTER T2      →  in dump NO,  in WAL YES  → CDC covers it
```

**No gap**: Slot (T1) is created before dump (T2). WAL captures everything the dump misses.

**No duplication**: Overlap rows use `INSERT ... ON CONFLICT (id) DO UPDATE` — same row written twice = one row.

---

## Containers

| Container | Folder | Port | Role |
|---|---|---|---|
| postgres1 | 1-reader/ | 5433 | Source OME database |
| kafka | 2-kafka-debezium/ | 29093 | Kafka broker (KRaft, no Zookeeper) |
| debezium | 2-kafka-debezium/ | 8083 | Reads WAL → Kafka |
| redpanda-console | 2-kafka-debezium/ | **8080** | **UI — open http://localhost:8080** |
| postgres2 | 3-writer/ | 5434 | Target Federation database |
| writer | 3-writer/ | — | Go: dump + CDC consumer |

## Live Test

After writer prints `WRITER COMPLETE`:

```bash
# Insert into source
podman exec postgres1 psql -U postgres -d omedb `
  -c "INSERT INTO devices(service_tag,device_name,device_type_id,model,ip_address,health_status) `
      VALUES('MYTEST','test.local',1,'PowerEdge R750','10.99.1.1','OK');"

# Check target (~3 seconds)
podman exec postgres2 psql -U postgres -d omedb `
  -c "SELECT id,service_tag,model,health_status FROM devices WHERE service_tag='MYTEST';"
```

## Project Structure

```
wal-cdc-replication-poc/
│
├── 1-reader/                          ← SOURCE (postgres1 — the database being read)
│   ├── deployments/postgres/init/
│   │   ├── 01-schema-and-data.sql     ← 13 OME tables + ~8500 rows seed data
│   │   └── pg_hba.conf               ← PostgreSQL host-based authentication
│   ├── docker-compose.yml             ← postgres1 only (postgres:17-bookworm)
│   └── README.md
│
├── 2-kafka-debezium/                  ← PIPELINE (reads WAL, produces Kafka events)
│   ├── connector.json                 ← Debezium connector configuration (reference)
│   ├── docker-compose.yml             ← Kafka 4.1.1 (KRaft) + Debezium 3.4 + Redpanda Console v2.8.10
│   └── README.md
│
├── 3-writer/                          ← WRITER (consumes Kafka, writes to postgres2)
│   ├── main.go                        ← Orchestrator: 11-step startup sequence + keep-alive loop
│   ├── config.go                      ← Constants, DSNs, table list, shared counters
│   ├── waiters.go                     ← Service readiness: waitForPG, waitForKafka, waitForDebezium
│   ├── replication.go                 ← WAL slot creation, pg_dump, pg_restore
│   ├── connector.go                   ← Debezium connector deployment + status polling
│   ├── consumer.go                    ← Kafka partition reader → upsert/delete into postgres2
│   ├── verify.go                      ← Test data insertion, row checks, timestamp comparison
│   ├── go.mod                         ← Go module dependencies
│   ├── Dockerfile                     ← Multi-stage build (Go 1.23 builder + postgres:17 runtime)
│   ├── docker-compose.yml             ← postgres2 + writer service
│   └── README.md
│
├── docs/
│   ├── DEEP_DIVE.md                   ← Detailed explanation of every config and code line
│   └── CONTRIBUTING.md                ← DCO, coding guidelines, PR process
│
├── scripts/
│   ├── start-all.ps1                  ← PowerShell: start all services in order
│   ├── stop-all.ps1                   ← PowerShell: tear down everything
│   ├── verify.ps1                     ← PowerShell: compare source vs target + live test
│   ├── start-all.sh                   ← Bash equivalent
│   ├── stop-all.sh
│   └── verify.sh
│
├── .gitignore
└── README.md                          ← Quick start and architecture overview
```

## Cleanup

```powershell
.\scripts\stop-all.ps1
```
