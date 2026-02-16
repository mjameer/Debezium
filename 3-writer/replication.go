// replication.go — WAL slot creation, pg_dump, and pg_restore operations.
// These functions handle the initial bulk data transfer from postgres1 to postgres2.
package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

// createSlot recreates the logical replication slot on postgres1 with the
// configured name and pgoutput plugin, returning the LSN at which it starts.
// This is T1 in the zero-loss timeline: everything after this LSN is captured.
func createSlot() string {
	db, _ := sql.Open("postgres", pg1DSN)
	defer db.Close()

	// Drop existing slot if any (idempotent restart)
	db.Exec(fmt.Sprintf(`DO $$ BEGIN
		IF EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name='%s')
		THEN PERFORM pg_drop_replication_slot('%s');
		END IF; END $$;`, slotName, slotName))

	var n, l string
	if err := db.QueryRow(fmt.Sprintf(
		"SELECT slot_name,lsn::TEXT FROM pg_create_logical_replication_slot('%s','pgoutput')",
		slotName)).Scan(&n, &l); err != nil {
		log.Fatalf("  Slot failed: %v", err)
	}
	log.Printf("  Slot '%s' at LSN %s", n, l)
	return l
}

// pgDump executes pg_dump against postgres1 for database omedb, writing a
// custom-format dump file on disk and returning its path and duration.
// This is T2 in the zero-loss timeline: the MVCC snapshot sees all committed data.
func pgDump() (string, time.Duration) {
	f := "/tmp/omedb.dump"
	start := time.Now()
	cmd := exec.Command("pg_dump",
		"-h", "postgres1", "-U", "postgres", "-d", "omedb",
		"-F", "c", "-f", f, "--no-owner", "--no-privileges")
	cmd.Env = append(os.Environ(), "PGPASSWORD=postgres")
	var se bytes.Buffer
	cmd.Stderr = &se
	if err := cmd.Run(); err != nil {
		log.Fatalf("  pg_dump: %v\n%s", err, se.String())
	}
	d := time.Since(start)
	fi, _ := os.Stat(f)
	log.Printf("  Dump: %d bytes in %v", fi.Size(), d)
	return f, d
}

// pgRestore recreates the omedb database on postgres2 and restores the
// provided dump file into it, returning the time taken.
func pgRestore(df string) time.Duration {
	start := time.Now()
	db, _ := sql.Open("postgres", pg2AdminDSN)
	db.Exec("DROP DATABASE IF EXISTS omedb")
	db.Exec("CREATE DATABASE omedb")
	db.Close()

	cmd := exec.Command("pg_restore",
		"-h", "postgres2", "-U", "postgres", "-d", "omedb",
		"--no-owner", "--no-privileges", df)
	cmd.Env = append(os.Environ(), "PGPASSWORD=postgres")
	cmd.Run()

	d := time.Since(start)
	log.Printf("  Restore: %v", d)
	return d
}
