
# Change Data Capture with Debezium - Summary

## Introduction
This README covers the key points about **Debezium**, a technology open-sourced by Red Hat in 2016, enabling efficient **Change Data Capture (CDC)**. CDC is a method to stream database changes in real-time, allowing them to be consumed downstream by various systems.

---

## What is Debezium?
Debezium uses **log-based CDC** to capture changes from a source database and stream them to downstream systems in a **database-agnostic format** (e.g., JSON). This avoids performance hits associated with other methods like:
- **Polling** (high resource usage).
- **Triggers** (consistency and performance issues).

---

## Key Features
- **Source & Sink Support**: Supports popular databases like MySQL, PostgreSQL, and any JDBC-compatible systems for sinks.
- **Database-Agnostic Events**: Ensures compatibility across heterogeneous systems.
- **Integration with Kafka**: For message brokering, enabling:
  - Asynchronous processing.
  - At-least-once message delivery.
  - Replayability for failure scenarios.

---

## Architecture
Debezium operates in two main components:
1. **Source Connector**: Reads database logs (e.g., Write-Ahead Logs) to extract changes.
2. **Sink Connector**: Sends processed changes to downstream systems (e.g., Elasticsearch, Redis).

These connectors use **Kafka Connect** for communication and decoupling.

---

## Features and Benefits
### Data Snapshotting
- Handles initial state by performing full/partial table snapshots.
- Useful for syncing databases that have already compacted logs.

### Use Cases
1. **Read-Optimized Views**: Synchronize data from a write-optimized source to a read-optimized target like data warehouses or search indexes.
2. **Audit Logs**: Retain a historical record of all database changes.
3. **Cache Consistency**: Update caches like Redis with minimal delay.
4. **CQRS (Command Query Responsibility Segregation)**: Maintain separate read/write databases for better performance.
5. **Avoid Dual Writes**: Ensure consistency across multiple databases without two-phase commits.
6. **Microservices**: Enable eventual consistency between microservices without direct coupling.

---

## Why Use Kafka?
- **Decouples Processing**: Ensures the source is not responsible for managing multiple downstream connections.
- **Persistence**: Logs events for replayability in case of downstream failures.
- **Scalability**: Supports multiple consumers and efficient message delivery.

---

## Conclusion
Debezium provides a low-overhead, efficient way to implement CDC with various downstream applications. By leveraging Kafka, it ensures robustness, scalability, and fault tolerance. Use cases span across data warehousing, audit logging, microservices, and cache management.

---

For further information, visit the [Debezium documentation](https://debezium.io/).

## Reference
- https://youtu.be/6VbRlQ0rL3I?si=EUIfbHUXrmN4sF_X
- https://www.baeldung.com/debezium-intro
- https://www.youtube.com/watch?v=n0m6r0kXZh8
- https://www.youtube.com/watch?v=n0m6r0kXZh8
