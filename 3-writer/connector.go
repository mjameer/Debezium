// connector.go — Debezium connector deployment and status monitoring.
// Handles POST-ing the connector JSON config and waiting for RUNNING state.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// tableIncludeList builds the Debezium table.include.list string for all
// tracked tables in the public schema.
func tableIncludeList() string {
	var parts []string
	for _, t := range tables {
		parts = append(parts, "public."+t)
	}
	return strings.Join(parts, ",")
}

// deployConnector posts the Debezium Postgres connector configuration to the
// Connect REST API, exiting fatally if deployment fails.
//
// Key config choices:
//   - snapshot.mode=never      → we did pg_dump ourselves, no Debezium snapshot
//   - time.precision.mode=isostring → timestamps as ISO-8601 strings (Debezium 3.1+)
//   - decimal.handling.mode=string  → no precision loss on NUMERIC columns
func deployConnector() {
	cfg := map[string]interface{}{
		"name": "ome-source",
		"config": map[string]interface{}{
			"connector.class":   "io.debezium.connector.postgresql.PostgresConnector",
			"database.hostname": "postgres1", "database.port": "5432",
			"database.user": "postgres", "database.password": "postgres", "database.dbname": "omedb",
			"topic.prefix": "ome", "slot.name": slotName, "plugin.name": "pgoutput",
			"publication.name": "dbz_publication", "snapshot.mode": "never",
			"table.include.list":             tableIncludeList(),
			"key.converter":                  "org.apache.kafka.connect.json.JsonConverter",
			"key.converter.schemas.enable":   "false",
			"value.converter":                "org.apache.kafka.connect.json.JsonConverter",
			"value.converter.schemas.enable": "false",
			"tombstones.on.delete":           "false",
			"decimal.handling.mode":          "string",
			"time.precision.mode":            "isostring",
		},
	}
	b, _ := json.Marshal(cfg)
	r, err := http.Post(debeziumURL+"/connectors", "application/json", bytes.NewReader(b))
	if err != nil {
		log.Fatalf("  Deploy: %v", err)
	}
	defer r.Body.Close()
	rb, _ := io.ReadAll(r.Body)
	if r.StatusCode == 201 || r.StatusCode == 200 {
		log.Println("  Connector deployed")
	} else {
		log.Fatalf("  Deploy error (%d): %s", r.StatusCode, string(rb))
	}
}

// waitForConnector polls the Debezium connector status endpoint until the
// ome-source connector reports RUNNING, or exits fatally on timeout.
func waitForConnector() {
	for i := 0; i < 60; i++ {
		r, err := http.Get(debeziumURL + "/connectors/ome-source/status")
		if err == nil {
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			var s map[string]interface{}
			json.Unmarshal(b, &s)
			if c, ok := s["connector"].(map[string]interface{}); ok {
				if st, _ := c["state"].(string); st == "RUNNING" {
					log.Println("  Connector RUNNING")
					return
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	log.Fatal("  Connector timeout")
}
