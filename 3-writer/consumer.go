// consumer.go — Kafka CDC consumer and postgres2 writer.
// Each table gets its own goroutine reading from Kafka partition 0 and applying
// upserts/deletes to postgres2 via ON CONFLICT.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// consumeAndWrite continuously consumes CDC events from the given Kafka topic
// (partition 0) and applies them to the specified postgres2 table, reconnecting
// the reader on errors.
func consumeAndWrite(ctx context.Context, topic, table string) {
	log.Printf("  [consumer] %s -> %s", topic, table)

	// Use direct partition reader instead of consumer groups.
	// Consumer groups with kafka-go + KRaft can have rebalance issues.
	// Direct partition 0 reader is simpler and reliable for single-broker.
	for {
		r := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   []string{kafkaBroker},
			Topic:     topic,
			Partition: 0,
			MinBytes:  1,
			MaxBytes:  10e6,
			MaxWait:   1 * time.Second,
		})
		r.SetOffset(kafka.FirstOffset)

		for {
			msg, err := r.ReadMessage(ctx)
			if err != nil {
				log.Printf("  [consumer] %s read error: %v", table, err)
				r.Close()
				time.Sleep(2 * time.Second)
				break
			}
			applyToPostgres2(table, msg.Value)
		}
	}
}

// applyToPostgres2 interprets a Debezium CDC message value, derives the op,
// before, and after payload fields, and performs an upsert or delete on
// postgres2 accordingly.
func applyToPostgres2(table string, value []byte) {
	var ev map[string]interface{}
	if json.Unmarshal(value, &ev) != nil {
		return
	}
	p := ev
	if pp, ok := ev["payload"].(map[string]interface{}); ok {
		p = pp
	}
	op, _ := p["op"].(string)
	after, _ := p["after"].(map[string]interface{})
	before, _ := p["before"].(map[string]interface{})

	db, err := sql.Open("postgres", pg2DSN)
	if err != nil {
		return
	}
	defer db.Close()

	switch op {
	case "c", "r", "u":
		if after != nil {
			upsert(db, table, after)
		}
	case "d":
		if before != nil {
			del(db, table, before)
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// TIMESTAMP AUTO-DETECTION FROM SCHEMA
// ═══════════════════════════════════════════════════════════════

// tsCache caches the set of timestamp column names per table so we only
// query information_schema once per table.
var (
	tsCache   = make(map[string]map[string]bool)
	tsCacheMu sync.Mutex
)

// getTimestampColumns returns a set of column names that are timestamp/date
// types for the given table. Results are cached after first lookup.
func getTimestampColumns(db *sql.DB, table string) map[string]bool {
	tsCacheMu.Lock()
	defer tsCacheMu.Unlock()

	if cols, ok := tsCache[table]; ok {
		return cols
	}

	cols := make(map[string]bool)
	rows, err := db.Query(`
		SELECT column_name FROM information_schema.columns
		WHERE table_schema='public' AND table_name=$1
		  AND data_type IN ('timestamp without time zone','timestamp with time zone','date')`, table)
	if err != nil {
		tsCache[table] = cols
		return cols
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		rows.Scan(&name)
		cols[name] = true
	}
	tsCache[table] = cols
	return cols
}

// convertTimestamp converts Debezium epoch-encoded temporal values to Go time.Time.
//
// Debezium 3.4 with time.precision.mode=isostring sends timestamps as
// ISO-8601 strings (e.g. "2026-02-16T21:04:46.955Z") which PostgreSQL accepts
// natively. This function handles the fallback case where the value is still
// numeric (e.g. if isostring mode was not applied for some fields).
func convertTimestamp(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		epoch := int64(n)
		if epoch == 0 {
			return v
		}
		// Try as microseconds first: if year < 2000, it's actually milliseconds
		asUsec := time.Unix(epoch/1_000_000, (epoch%1_000_000)*1000)
		if asUsec.Year() >= 2000 && asUsec.Year() <= 2100 {
			return asUsec // It's microseconds
		}
		// Otherwise treat as milliseconds
		return time.Unix(epoch/1_000, (epoch%1_000)*1_000_000)
	default:
		return v // strings (ISO-8601) pass through — PG accepts them natively
	}
}

// ═══════════════════════════════════════════════════════════════
// UPSERT + DELETE
// ═══════════════════════════════════════════════════════════════

// upsert builds and executes an INSERT ... ON CONFLICT(id) DO UPDATE statement
// for the given table and row data, auto-detecting timestamp columns from the
// schema and converting Debezium epoch values to time.Time.
func upsert(db *sql.DB, table string, data map[string]interface{}) {
	id := data["id"]
	tsCols := getTimestampColumns(db, table)

	var cols, phs, ups []string
	var vals []interface{}
	i := 1
	for k, v := range data {
		cols = append(cols, fmt.Sprintf(`"%s"`, k))
		phs = append(phs, fmt.Sprintf("$%d", i))
		if k != "id" {
			ups = append(ups, fmt.Sprintf(`"%s"=$%d`, k, i))
		}
		// Auto-convert timestamp columns detected from schema
		if tsCols[k] {
			vals = append(vals, convertTimestamp(v))
		} else {
			vals = append(vals, v)
		}
		i++
	}
	if len(ups) == 0 {
		return
	}
	q := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (id) DO UPDATE SET %s`,
		table, strings.Join(cols, ","), strings.Join(phs, ","), strings.Join(ups, ","))
	if _, err := db.Exec(q, vals...); err != nil {
		log.Printf("  [writer] upsert %s id=%v: %v", table, id, err)
	} else {
		atomic.AddInt64(&written, 1)
		log.Printf("  [writer] synced %s id=%v", table, id)
	}
}

// del issues a DELETE statement on postgres2 for the given table and primary
// key id, incrementing the CDC event counter.
func del(db *sql.DB, table string, data map[string]interface{}) {
	id := data["id"]
	db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id=$1", table), id)
	atomic.AddInt64(&written, 1)
	log.Printf("  [writer] deleted %s id=%v", table, id)
}
