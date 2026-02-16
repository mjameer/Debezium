// main.go — Orchestrator for the WAL CDC writer service.
// This file contains only the startup sequence and keep-alive loop.
// All logic is delegated to purpose-specific files:
//
//   config.go      — Constants, table list, shared state
//   waiters.go     — Service readiness checks (PG, Kafka, Debezium)
//   replication.go — Slot creation, pg_dump, pg_restore
//   connector.go   — Debezium connector deployment and status
//   consumer.go    — Kafka consumer → postgres2 writer (upsert/delete)
//   verify.go      — Test data insertion and verification
package main

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("╔══════════════════════════════════════════════════════════╗")
	log.Println("║  WRITER SERVICE                                         ║")
	log.Println("║  1. Creates replication slot on postgres1               ║")
	log.Println("║  2. pg_dump postgres1 → pg_restore postgres2            ║")
	log.Println("║  3. Deploys Debezium connector                          ║")
	log.Println("║  4. Consumes Kafka CDC events → writes to postgres2     ║")
	log.Println("╚══════════════════════════════════════════════════════════╝")

	// ── Wait for all services ─────────────────────────
	log.Println("\n[STEP 1] Waiting for postgres1 (source)...")
	waitForPG("postgres1", pg1DSN, "devices")
	logCounts("postgres1 BEFORE", pg1DSN)

	log.Println("[STEP 2] Waiting for postgres2 (target)...")
	waitForHost("postgres2")

	log.Println("[STEP 3] Waiting for Kafka...")
	waitForKafka()

	log.Println("[STEP 4] Waiting for Debezium...")
	waitForDebezium()

	// ── Create slot THEN dump ─────────────────────────
	log.Println("\n[STEP 5] Creating replication slot on postgres1...")
	log.Println("  This bookmarks the WAL. Everything from here is captured.")
	slotLSN := createSlot()

	log.Println("\n[STEP 6] pg_dump from postgres1...")
	dumpFile, dumpDur := pgDump()

	log.Println("\n[STEP 7] pg_restore into postgres2...")
	restoreDur := pgRestore(dumpFile)
	logCounts("postgres2 AFTER RESTORE", pg2DSN)

	// ── Deploy Debezium connector ─────────────────────
	log.Println("\n[STEP 8] Deploying Debezium connector...")
	log.Println("  snapshot.mode=never — Debezium reads WAL from slot, no re-snapshot")
	deployConnector()
	waitForConnector()

	// ── Start Kafka consumers → write to postgres2 ────
	log.Println("\n[STEP 9] Starting Kafka consumers → postgres2...")
	ctx := context.Background()
	for _, t := range tables {
		go consumeAndWrite(ctx, "ome.public."+t, t)
	}
	log.Printf("  Started %d consumers", len(tables))
	time.Sleep(8 * time.Second)

	// ── Test: insert into postgres1, verify in postgres2
	log.Println("\n[STEP 10] Inserting test data into postgres1 (post-dump)...")
	insertTestData()

	log.Println("[STEP 11] Waiting 15s for CDC...")
	time.Sleep(15 * time.Second)
	verify()

	// ── Summary ───────────────────────────────────────
	log.Println("\n══════════════════════════════════════════════════════════")
	log.Println("  WRITER COMPLETE")
	log.Println("══════════════════════════════════════════════════════════")
	log.Printf("  Slot LSN:     %s", slotLSN)
	log.Printf("  Dump:         %v", dumpDur)
	log.Printf("  Restore:      %v", restoreDur)
	log.Printf("  CDC applied:  %d events", atomic.LoadInt64(&written))
	logCounts("postgres1 FINAL", pg1DSN)
	logCounts("postgres2 FINAL", pg2DSN)
	log.Println("\n  TRY LIVE:")
	log.Println(`  podman exec postgres1 psql -U postgres -d omedb -c "INSERT INTO devices(service_tag,device_name,device_type_id,model,ip_address,health_status) VALUES('LIVETEST','live.local',1,'PowerEdge R750','10.99.99.1','OK');"`)
	log.Println(`  podman exec postgres2 psql -U postgres -d omedb -c "SELECT id,service_tag,model,health_status FROM devices WHERE service_tag='LIVETEST';"`)
	log.Println("══════════════════════════════════════════════════════════")

	// Keep alive — consumers run in background goroutines
	for {
		time.Sleep(10 * time.Second)
		log.Printf("[writer] CDC events written: %d", atomic.LoadInt64(&written))
	}
}
