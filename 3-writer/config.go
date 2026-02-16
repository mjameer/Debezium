// config.go — Constants, table lists, and shared state for the writer service.
package main

// Connection strings and service endpoints.
const (
	pg1DSN      = "postgres://postgres:postgres@postgres1:5432/omedb?sslmode=disable"
	pg2DSN      = "postgres://postgres:postgres@postgres2:5432/omedb?sslmode=disable"
	pg2AdminDSN = "postgres://postgres:postgres@postgres2:5432/postgres?sslmode=disable"
	kafkaBroker = "kafka:9092"
	debeziumURL = "http://debezium:8083"
	slotName    = "debezium_slot"
)

// tables is the ordered list of OME tables to replicate.
// To add/remove tables from replication, edit this list.
// All four layers must agree: publication, Debezium include list, this list, pg_dump.
var tables = []string{
	"device_types", "devices", "device_inventory", "groups", "group_memberships",
	"alert_categories", "alerts", "device_health", "firmware_catalog",
	"compliance_baselines", "compliance_results", "users", "job_history",
}

// written tracks the total number of CDC events successfully applied to postgres2.
// Accessed atomically from multiple goroutines (one per table consumer).
var written int64
