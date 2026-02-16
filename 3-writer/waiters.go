// waiters.go — Service readiness checks for PostgreSQL, Kafka, and Debezium.
// These functions block until each dependency is reachable, or exit fatally on timeout.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// waitForPG blocks until the given PostgreSQL instance is reachable and the
// specified table has at least one row, or exits fatally on timeout.
func waitForPG(name, dsn, table string) {
	for i := 0; i < 120; i++ {
		db, err := sql.Open("postgres", dsn)
		if err == nil {
			var n int
			err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
			db.Close()
			if err == nil && n > 0 {
				log.Printf("  %s ready (%d %s)", name, n, table)
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("  %s timeout", name)
}

// waitForHost waits until a Postgres server at the given host responds to Ping,
// indicating the service is up, or exits fatally on timeout.
func waitForHost(name string) {
	dsn := fmt.Sprintf("postgres://postgres:postgres@%s:5432/postgres?sslmode=disable", name)
	for i := 0; i < 60; i++ {
		db, err := sql.Open("postgres", dsn)
		if err == nil {
			err = db.Ping()
			db.Close()
			if err == nil {
				log.Printf("  %s ready", name)
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("  %s timeout", name)
}

// waitForKafka waits until a TCP connection to the configured Kafka broker
// succeeds, indicating Kafka is available, or exits fatally on timeout.
func waitForKafka() {
	for i := 0; i < 90; i++ {
		conn, err := kafka.Dial("tcp", kafkaBroker)
		if err == nil {
			conn.Close()
			log.Println("  Kafka ready")
			return
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatal("  Kafka timeout")
}

// waitForDebezium polls the Debezium Connect REST endpoint until it returns
// HTTP 200 for /connectors, signaling that Debezium is ready, or exits on timeout.
func waitForDebezium() {
	for i := 0; i < 120; i++ {
		r, err := http.Get(debeziumURL + "/connectors")
		if err == nil && r.StatusCode == 200 {
			r.Body.Close()
			log.Println("  Debezium ready")
			return
		}
		if r != nil {
			r.Body.Close()
		}
		time.Sleep(3 * time.Second)
	}
	log.Fatal("  Debezium timeout")
}
