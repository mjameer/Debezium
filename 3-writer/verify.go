// verify.go — Test data insertion and verification.
// Inserts test rows into postgres1 post-dump, then checks they arrive in postgres2.
// Also compares timestamp values between source and target for accuracy.
package main

import (
	"database/sql"
	"log"
	"time"
)

// insertTestData writes a small set of inserts and an update into postgres1
// after the initial dump to validate that Debezium CDC propagates changes.
func insertTestData() {
	db, _ := sql.Open("postgres", pg1DSN)
	defer db.Close()
	for _, t := range []struct{ d, q string }{
		{"INSERT device CDCSRV01", `INSERT INTO devices(service_tag,device_name,device_type_id,model,ip_address,health_status,ome_device_id) VALUES('CDCSRV01','cdc-test.local',2,'PowerEdge R760','10.200.1.1','OK',99001)`},
		{"INSERT alert", `INSERT INTO alerts(device_id,alert_category_id,severity,message,source) VALUES(1,1,'Warning','CDC test: memory ECC error','iDRAC')`},
		{"INSERT group", `INSERT INTO groups(name,description,group_type,created_by) VALUES('CDC-Test-Group','Created post-dump','Static','admin')`},
		{"UPDATE device 1", `UPDATE devices SET health_status='Warning',updated_at=NOW() WHERE id=1`},
	} {
		if _, err := db.Exec(t.q); err != nil {
			log.Printf("  WARN %s: %v", t.d, err)
		} else {
			log.Printf("  + %s", t.d)
		}
	}
}

// verify checks postgres2 for the presence of the test records, updated
// device row, and timestamp accuracy between source and target.
func verify() {
	db, _ := sql.Open("postgres", pg2DSN)
	defer db.Close()
	log.Println("[verify] Checking postgres2:")

	// 1. Row existence checks
	for _, c := range []struct{ d, q string }{
		{"device CDCSRV01", "SELECT COUNT(*) FROM devices WHERE service_tag='CDCSRV01'"},
		{"CDC alert", "SELECT COUNT(*) FROM alerts WHERE message LIKE 'CDC test%'"},
		{"CDC-Test-Group", "SELECT COUNT(*) FROM groups WHERE name='CDC-Test-Group'"},
		{"device 1 updated", "SELECT COUNT(*) FROM devices WHERE id=1 AND health_status='Warning'"},
	} {
		var n int
		db.QueryRow(c.q).Scan(&n)
		if n > 0 {
			log.Printf("  PASS: %s", c.d)
		} else {
			log.Printf("  PENDING: %s", c.d)
		}
	}

	// 2. Timestamp accuracy verification
	log.Println("[verify] Timestamp comparison (source vs target):")
	verifyTimestamps()
}

// verifyTimestamps compares actual timestamp values between postgres1 and postgres2
// for a sample of rows across multiple tables, logging MATCH or MISMATCH for each.
func verifyTimestamps() {
	db1, err := sql.Open("postgres", pg1DSN)
	if err != nil {
		log.Printf("  SKIP: cannot connect to postgres1: %v", err)
		return
	}
	defer db1.Close()

	db2, err := sql.Open("postgres", pg2DSN)
	if err != nil {
		log.Printf("  SKIP: cannot connect to postgres2: %v", err)
		return
	}
	defer db2.Close()

	checks := []struct {
		table string
		query string
	}{
		{"devices", "SELECT id, created_at, updated_at FROM devices WHERE id IN (1,2,3,100,500) ORDER BY id"},
		{"alerts", "SELECT id, alert_time, created_at FROM alerts WHERE id IN (1,50,100,500,1000) ORDER BY id"},
		{"device_health", "SELECT id, collected_at FROM device_health WHERE id IN (1,100,250,500) ORDER BY id"},
		{"job_history", "SELECT id, started_at, completed_at, created_at FROM job_history WHERE id IN (1,25,50,100) ORDER BY id"},
	}

	totalChecked := 0
	totalMatched := 0

	for _, c := range checks {
		rows1, err := db1.Query(c.query)
		if err != nil {
			log.Printf("  SKIP %s: %v", c.table, err)
			continue
		}

		rows2, err := db2.Query(c.query)
		if err != nil {
			rows1.Close()
			log.Printf("  SKIP %s: %v", c.table, err)
			continue
		}

		cols, _ := rows1.Columns()
		numCols := len(cols)

		for rows1.Next() && rows2.Next() {
			vals1 := make([]interface{}, numCols)
			vals2 := make([]interface{}, numCols)
			ptrs1 := make([]interface{}, numCols)
			ptrs2 := make([]interface{}, numCols)
			for i := range vals1 {
				ptrs1[i] = &vals1[i]
				ptrs2[i] = &vals2[i]
			}
			rows1.Scan(ptrs1...)
			rows2.Scan(ptrs2...)

			rowID := vals1[0]
			for i := 1; i < numCols; i++ {
				t1, ok1 := vals1[i].(time.Time)
				t2, ok2 := vals2[i].(time.Time)
				if ok1 && ok2 {
					totalChecked++
					diff := t1.Sub(t2)
					if diff < 0 {
						diff = -diff
					}
					if diff <= time.Second {
						totalMatched++
					} else {
						log.Printf("  MISMATCH %s id=%v col=%s: src=%v dst=%v diff=%v",
							c.table, rowID, cols[i], t1, t2, diff)
					}
				}
			}
		}
		rows1.Close()
		rows2.Close()
	}

	if totalChecked > 0 {
		log.Printf("  Timestamps: %d/%d matched (%.1f%% accuracy)",
			totalMatched, totalChecked, float64(totalMatched)/float64(totalChecked)*100)
		if totalMatched == totalChecked {
			log.Println("  PASS: All timestamps match between source and target")
		}
	} else {
		log.Println("  SKIP: No timestamp rows to compare (CDC data may not be applied yet)")
	}
}

// logCounts prints per-table and total row counts for all tracked tables
// in the database identified by the given DSN, labeled for context.
func logCounts(label, dsn string) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return
	}
	defer db.Close()
	log.Printf("[%s]:", label)
	total := 0
	for _, t := range tables {
		var c int
		db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&c)
		total += c
		log.Printf("  %-25s %d", t, c)
	}
	log.Printf("  %-25s %d", "TOTAL", total)
}
