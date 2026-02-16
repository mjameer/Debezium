# 3-writer — Target Database + Go Consumer

**What**: postgres2 (empty target) + Go service that writes to it.

**Role**: This is the **WRITER**. It:

1. Creates a replication slot on postgres1 (bookmarks WAL position)
2. `pg_dump` postgres1 → `pg_restore` postgres2 (bulk copy)
3. Deploys the Debezium connector (tells Debezium to start reading WAL)
4. Consumes CDC events from Kafka → upserts into postgres2

**Reads from**: Kafka topics (produced by Debezium in `2-kafka-debezium/`)
**Writes to**: postgres2

## Run (after 1-reader and 2-kafka-debezium are up)

```bash
cd 3-writer
podman compose up --build
```

## Services

| Container | Port | Role |
|---|---|---|
| postgres2 | 5434 | Target Federation database (starts empty) |
| writer | — | Go: slot → dump → restore → consume Kafka → upsert |
