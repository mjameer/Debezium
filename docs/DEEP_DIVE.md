
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

## Adding New Tables

To add a new table to the replication:

1. **Create the table** in `1-reader/deployments/postgres/init/01-schema-and-data.sql`
2. **Add to the publication** (if using `FOR TABLE` instead of `FOR ALL TABLES`)
3. **Add to the Go table list** in `3-writer/main.go` → `var tables = []string{...}`
4. Rebuild and restart: `.\scripts\stop-all.ps1` then `.\scripts\start-all.ps1`

## Filtering Tables

To replicate only specific tables (e.g., 75 out of 100):

| Layer | File | Config |
|---|---|---|
| PostgreSQL Publication | `01-schema-and-data.sql` | `CREATE PUBLICATION ... FOR TABLE x,y,z` |
| Debezium | `connector.json` or `connector.go` | `table.include.list` |
| Go Writer | `config.go` | `var tables = []string{...}` |
| pg_dump | `replication.go` pgDump() | Add `-t table1 -t table2` flags |

All four layers should agree on which tables to replicate.



# Deep Dive: WAL CDC Replication POC

## Table of Contents

- [1. Table Filtering — How to Replicate Only 75 Out of 100 Tables](#1-table-filtering)
- [2. CREATE PUBLICATION — What It Is and Why](#2-create-publication)
- [3. pg_hba.conf — PostgreSQL Authentication](#3-pg_hbaconf)
- [4. Docker Compose Volumes and Command](#4-docker-compose-volumes-and-command)
- [5. Debezium Connector JSON Config](#5-debezium-connector-json-config)
- [6. Go Writer Code — Line by Line](#6-go-writer-code)
- [7. Zero Data Loss — How It Works](#7-zero-data-loss)

---

## 1. Table Filtering

> "If postgres1 has 100 tables but I only want 75 in postgres2, how?"

Filter at **four layers**. All four should agree.

### Layer 1: PostgreSQL Publication (source side)

```sql
-- CURRENT: captures ALL tables
CREATE PUBLICATION dbz_publication FOR ALL TABLES;

-- FILTERED: only the 75 you want
CREATE PUBLICATION dbz_publication FOR TABLE
    devices,
    alerts,
    device_health,
    groups,
    -- ... list your 75 tables here
    job_history;
```

This controls what PostgreSQL writes to the WAL for logical replication consumers.
Tables not in the publication produce no WAL events for Debezium.

### Layer 2: Debezium Connector (pipeline side)

In `connector.json` or in the Go code that deploys the connector:

```json
"table.include.list": "public.devices,public.alerts,public.device_health,..."
```

Only tables listed here get CDC events published to Kafka topics.
Even if the publication includes 100 tables, Debezium ignores the other 25.

You can also use `table.exclude.list` to blacklist instead:

```json
"table.exclude.list": "public.audit_log,public.temp_data,..."
```

### Layer 3: Go Writer (consumer side)

```go
var tables = []string{
    "devices", "alerts", "device_health",
    // ... your 75 tables
}
```

This controls which Kafka topics the writer subscribes to.
One goroutine per table. No goroutine = no consumption.

### Layer 4: pg_dump (bulk copy)

```bash
# Dump only specific tables
pg_dump -h postgres1 -U postgres -d omedb \
    -t devices -t alerts -t device_health \
    ... \
    -F c -f /tmp/omedb.dump
```

Or exclude tables:

```bash
pg_dump -h postgres1 -U postgres -d omedb \
    -T audit_log -T temp_data \
    -F c -f /tmp/omedb.dump
```

### Summary

| Layer | Config | Controls |
|---|---|---|
| Publication | `CREATE PUBLICATION ... FOR TABLE x,y,z` | What WAL events PostgreSQL produces |
| Debezium | `table.include.list` | What WAL events Debezium reads and publishes to Kafka |
| Writer Go code | `var tables = []string{...}` | Which Kafka topics the writer consumes |
| pg_dump | `-t table1 -t table2` or `-T excluded` | Which tables are in the initial bulk dump |

---

## 2. CREATE PUBLICATION

```sql
CREATE PUBLICATION dbz_publication FOR ALL TABLES;
```

### What is it?

A **publication** is a PostgreSQL feature (introduced in PG 10) that defines which tables
participate in **logical replication**. It tells PostgreSQL: "write detailed row-level
change events to the WAL for these tables."

### Why is it needed?

Without a publication, PostgreSQL writes minimal WAL entries (enough for crash recovery
but not enough to reconstruct row changes). With a publication, the WAL includes:

- The full row data for INSERTs
- The before/after row data for UPDATEs
- The row identity for DELETEs

Debezium reads these detailed WAL entries through the logical replication protocol.

### Variants

```sql
-- All tables (simple, good for POC)
CREATE PUBLICATION my_pub FOR ALL TABLES;

-- Specific tables (production — explicit control)
CREATE PUBLICATION my_pub FOR TABLE devices, alerts, device_health;

-- Only INSERT and UPDATE, no DELETE
CREATE PUBLICATION my_pub FOR TABLE devices WITH (publish = 'insert,update');
```

### Where is it used?

In the Debezium connector config:

```json
"publication.name": "dbz_publication"
```

Debezium connects to this publication to know which tables to read WAL for.

---

## 3. pg_hba.conf

```
local   all             all                                     trust
host    all             all             0.0.0.0/0               md5
host    replication     all             0.0.0.0/0               md5
```

### What is it?

`pg_hba.conf` = **Host-Based Authentication**. It is PostgreSQL's access control file.
Every connection attempt is checked against these rules top-to-bottom. First match wins.

### Line-by-line

```
local   all   all   trust
│       │     │     └── Auth method: "trust" = no password needed
│       │     └──────── User: "all" = any user
│       └────────────── Database: "all" = any database
└────────────────────── Connection type: "local" = Unix socket (inside container)
```

```
host    all   all   0.0.0.0/0   md5
│       │     │     │           └── Auth: "md5" = password required (hashed)
│       │     │     └────────────── IP range: 0.0.0.0/0 = any IP address
│       │     └──────────────────── User: any
│       └────────────────────────── Database: any
└────────────────────────────────── Connection type: "host" = TCP/IP
```

```
host    replication   all   0.0.0.0/0   md5
│       │             │     │           └── Auth: password required
│       │             │     └────────────── IP: any
│       │             └──────────────────── User: any
│       └────────────────────────────────── Database: "replication" (special)
└────────────────────────────────────────── Connection type: TCP/IP
```

### Why the "replication" line matters

Debezium uses PostgreSQL's **replication protocol** (not normal SQL) to read the WAL.
This is a special connection type. Without this line, Debezium gets:

```
FATAL: no pg_hba.conf entry for replication connection
```

### Production notes

In production, replace `0.0.0.0/0` with specific CIDRs:

```
host    replication   debezium_user   10.0.0.0/8   scram-sha-256
```

---

## 4. Docker Compose Volumes and Command

```yaml
volumes:
  - ./deployments/postgres/init/01-schema-and-data.sql:/docker-entrypoint-initdb.d/01-init.sql:Z
  - ./deployments/postgres/init/pg_hba.conf:/etc/postgresql/pg_hba.conf:Z
command: >
  postgres
    -c wal_level=logical
    -c max_replication_slots=10
    -c max_wal_senders=10
    -c hba_file=/etc/postgresql/pg_hba.conf
```

### Volumes

```yaml
- ./deployments/postgres/init/01-schema-and-data.sql:/docker-entrypoint-initdb.d/01-init.sql:Z
  │                                                  │                                       │
  │                                                  │                                       └── :Z = SELinux relabel
  │                                                  │                                           (required for Podman)
  │                                                  └── Target: PostgreSQL runs ALL .sql files
  │                                                      in /docker-entrypoint-initdb.d/ on FIRST
  │                                                      startup only. This creates our tables and
  │                                                      inserts seed data.
  └── Source: our SQL file on the host machine
```

```yaml
- ./deployments/postgres/init/pg_hba.conf:/etc/postgresql/pg_hba.conf:Z
  └── Mounts our custom authentication config into the container.
      The "command" below tells postgres to use THIS file.
```

### Command

```yaml
command: >
  postgres                                    # Run the postgres server with these overrides:
    -c wal_level=logical                      # CRITICAL: Enable logical WAL decoding.
                                              # Default is "replica" which only supports
                                              # physical replication. "logical" adds row-level
                                              # detail needed by Debezium.
                                              #
    -c max_replication_slots=10               # Allow up to 10 replication slots.
                                              # Each Debezium connector uses 1 slot.
                                              # Each slot = bookmark in the WAL.
                                              #
    -c max_wal_senders=10                     # Allow up to 10 concurrent WAL streaming
                                              # connections. Debezium uses 1 WAL sender.
                                              #
    -c hba_file=/etc/postgresql/pg_hba.conf   # Use OUR custom pg_hba.conf (the one we
                                              # mounted via volumes) instead of the default.
```

### Why wal_level=logical?

```
wal_level=minimal   → Crash recovery only. No replication.
wal_level=replica   → Physical replication (byte-level). Can't decode rows.
wal_level=logical   → Logical replication (row-level). Debezium needs this.
```

---

## 5. Debezium Connector JSON Config

```json
{
  "name": "ome-source",
```

**name**: Unique connector name. Shows in Debezium REST API and Redpanda Console.
You can have multiple connectors (e.g., one per OME instance).

```json
  "config": {
    "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
```

**connector.class**: Which Debezium connector plugin to use. Debezium has connectors
for PostgreSQL, MySQL, MongoDB, SQL Server, Oracle, etc. We use the PostgreSQL one.

```json
    "database.hostname": "postgres1",
    "database.port": "5432",
    "database.user": "postgres",
    "database.password": "postgres",
    "database.dbname": "omedb",
```

**database.\***: Connection details to the SOURCE database.
Debezium connects here to read the WAL. In production, use a dedicated
replication user with minimal permissions, not "postgres".

```json
    "topic.prefix": "ome",
```

**topic.prefix**: Kafka topic naming. Topics are named:
`{prefix}.{schema}.{table}` → `ome.public.devices`, `ome.public.alerts`, etc.

```json
    "slot.name": "debezium_slot",
```

**slot.name**: Which PostgreSQL replication slot to use.
We created this slot in Go BEFORE the dump (the zero-loss trick).
The slot tells PostgreSQL: "keep WAL from this LSN until I consume it."

```json
    "plugin.name": "pgoutput",
```

**plugin.name**: The WAL decoder plugin. `pgoutput` is built into PostgreSQL 10+.
Alternative: `decoderbufs` (requires a separate extension). Use `pgoutput`.

```json
    "publication.name": "dbz_publication",
```

**publication.name**: Which PostgreSQL publication to subscribe to.
Must match the `CREATE PUBLICATION` we ran in the SQL init script.

```json
    "snapshot.mode": "never",
```

**snapshot.mode**: CRITICAL setting.
- `"never"`: Don't snapshot. We already did pg_dump/pg_restore. Just read WAL.
- `"initial"`: Debezium would SELECT * from every table first (slow, redundant for us).
- `"schema_only"`: Capture schema but no data (used when you only want future changes).

We use `"never"` because our Go code handles the initial bulk copy.

```json
    "table.include.list": "public.devices,public.alerts,...",
```

**table.include.list**: FILTER. Only these tables get CDC events.
Format: `{schema}.{table}`. Comma-separated. This is how you filter 75 out of 100 tables.

```json
    "key.converter": "org.apache.kafka.connect.json.JsonConverter",
    "key.converter.schemas.enable": "false",
    "value.converter": "org.apache.kafka.connect.json.JsonConverter",
    "value.converter.schemas.enable": "false",
```

**key/value.converter**: How to serialize Kafka messages.
JsonConverter = plain JSON. `schemas.enable=false` means don't wrap every message
in a schema envelope (keeps messages smaller and simpler to parse in Go).

Without `schemas.enable=false`, every message would look like:

```json
{"schema": {...big schema...}, "payload": {...actual data...}}
```

With it disabled, you get just the data directly.

```json
    "tombstones.on.delete": "false",
```

**tombstones.on.delete**: When a row is deleted, Kafka can send a second message
with null value (a "tombstone") for log compaction. We don't need this.

```json
    "decimal.handling.mode": "string"
```

**decimal.handling.mode**: How to represent DECIMAL/NUMERIC columns.
- `"string"`: Send as "42.50" (safe, no precision loss)
- `"double"`: Send as 42.5 (can lose precision)
- `"precise"`: Send as base64-encoded BigDecimal (complex to parse)

---
