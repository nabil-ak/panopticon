// panopticon collects statistics posted to it, and records them in a sqlite3 database.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	dbPath = flag.String("db", "stats.db", "Path to sqlite3 database")
	port   = flag.Int("port", 9001, "Port on which to serve HTTP")
)

type StatsReport struct {
	Homeserver       string
	LocalTimestamp   int64  // Seconds since epoch, UTC
	RemoteTimestamp  *int64 `json:"timestamp"` // Seconds since epoch, UTC
	UptimeSeconds    *int64 `json:"uptime_seconds"`
	TotalUsers       *int64 `json:"total_users"`
	TotalRoomCount   *int64 `json:"total_room_count"`
	DailyActiveUsers *int64 `json:"daily_active_users"`
	DailyMessages    *int64 `json:"daily_messages"`
	RemoteAddr       string
}

func main() {
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Could not open database: %v", err)
	}
	defer db.Close()

	if err := createTable(db); err != nil {
		log.Fatalf("Error creating database: %v", err)
	}

	r := &Recorder{db}

	http.HandleFunc("/push", r.Handle)
	http.HandleFunc("/test", serveText("ok"))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

type Recorder struct {
	DB *sql.DB
}

func (r *Recorder) Handle(w http.ResponseWriter, req *http.Request) {
	dec := json.NewDecoder(req.Body)
	defer req.Body.Close()
	var sr StatsReport
	if err := dec.Decode(&sr); err != nil {
		logAndReplyError(w, err, 400, "Error decoding JSON")
		return
	}
	sr.LocalTimestamp = time.Now().UTC().Unix()
	sr.RemoteAddr = req.RemoteAddr
	if err := r.Save(sr); err != nil {
		logAndReplyError(w, err, 500, "Error saving to DB")
		return
	}
	io.WriteString(w, "{}")
}

func (r *Recorder) Save(sr StatsReport) error {
	cols := []string{"homeserver", "local_timestamp", "remote_addr"}
	vals := []interface{}{sr.Homeserver, sr.LocalTimestamp, sr.RemoteAddr}
	if sr.RemoteTimestamp != nil {
		cols = append(cols, "remote_timestamp")
		vals = append(vals, *sr.RemoteTimestamp)
	}
	if sr.UptimeSeconds != nil {
		cols = append(cols, "uptime_seconds")
		vals = append(vals, *sr.UptimeSeconds)
	}
	if sr.TotalUsers != nil {
		cols = append(cols, "total_users")
		vals = append(vals, *sr.TotalUsers)
	}
	if sr.TotalRoomCount != nil {
		cols = append(cols, "total_room_count")
		vals = append(vals, *sr.TotalRoomCount)
	}
	if sr.DailyActiveUsers != nil {
		cols = append(cols, "daily_active_users")
		vals = append(vals, *sr.DailyActiveUsers)
	}
	if sr.DailyMessages != nil {
		cols = append(cols, "daily_messages")
		vals = append(vals, *sr.DailyMessages)
	}

	var valuePlaceholders []string
	for i := range vals {
		valuePlaceholders = append(valuePlaceholders, fmt.Sprintf("$%d", i+1))
	}
	_, err := r.DB.Exec(`INSERT INTO stats (
			`+strings.Join(cols, ", ")+`
		) VALUES (`+strings.Join(valuePlaceholders, ", ")+`)`,
		vals...,
	)
	return err
}

func logAndReplyError(w http.ResponseWriter, err error, code int, description string) {
	log.Printf("%s: %v", description, err)
	w.WriteHeader(code)
	io.WriteString(w, `{"error_message": "unable to process request"}`)
}

func serveText(s string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, s)
	}
}

func createTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS stats(
		id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		homeserver VARCHAR(256),
		local_timestamp BIGINT,
		remote_timestamp BIGINT,
		remote_addr TEXT,
		uptime_seconds BIGINT,
		total_users BIGINT,
		total_room_count BIGINT,
		daily_active_users BIGINT,
		daily_messages BIGINT
		)`)
	return err
}
