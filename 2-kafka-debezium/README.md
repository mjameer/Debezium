# 2-kafka-debezium — The CDC Pipeline

**What**: Kafka (KRaft mode, no Zookeeper) + Debezium Connect + Redpanda Console.

**Role**: The **pipeline** between reader and writer.

- Debezium **reads** postgres1's WAL (from `1-reader/`)
- Converts WAL entries into JSON CDC events
- Publishes them to Kafka topics (`ome.public.devices`, `ome.public.alerts`, etc.)
- The Writer (`3-writer/`) consumes from these topics
- Redpanda Console gives you a **browser UI** to see everything

## Run (after 1-reader is up)

```powershell
cd 2-kafka-debezium
py -m podman_compose up -d
```

## Redpanda Console (UI)

Open **http://localhost:8080** in your browser to:

- View all Kafka topics and their messages
- See CDC events flowing in real-time
- Monitor the Debezium connector status
- Inspect consumer groups and lag

## Deploy the Debezium connector

After Debezium is healthy:

```powershell
# Check Debezium is ready
curl http://localhost:8083/connectors

# Deploy connector (done automatically by the writer, but you can also do it manually)
curl -X POST http://localhost:8083/connectors -H "Content-Type: application/json" -d @connector.json

# Check status
curl http://localhost:8083/connectors/ome-source/status
```

## Services

| Container | Port | Role |
|---|---|---|
| kafka | 29093 (external) | Kafka broker (KRaft, no Zookeeper) |
| debezium | 8083 | Reads WAL → publishes to Kafka |
| redpanda-console | **8080** | **UI — open in browser** |
