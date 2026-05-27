package sparkdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

var errRateLimited = fmt.Errorf("rate limit exceeded")

type DB struct {
	mu            sync.Mutex
	client        *http.Client
	baseURL       string
	database      string
	token         string
	lastRequest   time.Time
	rlMu          sync.Mutex
	minRequestGap time.Duration
}

const defaultMinRequestGap = 6 * time.Millisecond

type Config struct {
	Host           string
	Port           int
	Database       string
	APIKey         string
	MinRequestGap  time.Duration
}

func Open(cfg Config) (*DB, error) {
	baseURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	gap := cfg.MinRequestGap
	if gap <= 0 {
		gap = defaultMinRequestGap
	}
	db := &DB{
		client:        &http.Client{Timeout: 30 * time.Second},
		baseURL:       baseURL,
		database:      cfg.Database,
		minRequestGap: gap,
	}
	if cfg.Database == "" {
		db.database = "deviceos"
	}

	req, _ := http.NewRequest("POST", baseURL+"/auth/login",
		bytes.NewReader([]byte(`{"username":"admin","password":"admin"}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := db.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sparkdb: connect to %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Token != "" {
		db.token = result.Token
	} else {
		req2, _ := http.NewRequest("POST", baseURL+"/auth/api-key",
			bytes.NewReader([]byte(`{"api_key":"`+cfg.APIKey+`"}`)))
		req2.Header.Set("Content-Type", "application/json")
		resp2, _ := db.client.Do(req2)
		if resp2 != nil {
			var r2 struct{ Token string `json:"token"` }
			json.NewDecoder(resp2.Body).Decode(&r2)
			db.token = r2.Token
			resp2.Body.Close()
		}
	}

	if db.token == "" {
		slog.Warn("sparkdb: no auth token acquired, queries may fail")
	}

	slog.Info("sparkdb connected", "server", baseURL, "database", db.database)
	return db, nil
}

func (db *DB) throttle() {
	db.rlMu.Lock()
	defer db.rlMu.Unlock()
	elapsed := time.Since(db.lastRequest)
	if elapsed < db.minRequestGap {
		time.Sleep(db.minRequestGap - elapsed)
	}
	db.lastRequest = time.Now()
}

func (db *DB) request(method, path string, body io.Reader) (*http.Response, error) {
	url := db.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if db.token != "" {
		req.Header.Set("Authorization", "Bearer "+db.token)
	}
	return db.client.Do(req)
}

type queryRequest struct {
	Query    string        `json:"query"`
	Database string        `json:"database"`
	Params   []interface{} `json:"params,omitempty"`
}

type queryResponse struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Error   string          `json:"error,omitempty"`
}

func (db *DB) query(sql string, params []interface{}) (*queryResponse, error) {
	body, _ := json.Marshal(queryRequest{
		Query:    sql,
		Database: db.database,
		Params:   params,
	})

	var lastErr error
	for attempt := range 3 {
		db.throttle()

		resp, err := db.request("POST", "/query", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("sparkdb: request: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// SparkDB returns "rate limit exceeded" as plain text (not JSON).
		// Retry with exponential backoff when we hit its internal rate limiter.
		if strings.Contains(string(raw), "rate limit exceeded") {
			lastErr = errRateLimited
			backoff := time.Duration(100*(attempt+1)*(attempt+1)) * time.Millisecond
			time.Sleep(backoff)
			continue
		}

		var qr queryResponse
		if err := json.Unmarshal(raw, &qr); err != nil {
			return nil, fmt.Errorf("sparkdb: decode: %w (body: %s)", err, string(raw))
		}
		if qr.Error != "" {
			return nil, fmt.Errorf("sparkdb: %s", qr.Error)
		}
		return &qr, nil
	}
	return nil, fmt.Errorf("sparkdb: %w after 3 attempts", lastErr)
}

// TxInterface defines transaction operations used by modules.
type TxInterface interface {
	Exec(sql string, args ...interface{}) (Result, error)
	Commit() error
	Rollback() error
}

// DBClient defines the database operations used by feature modules.
type DBClient interface {
	Exec(sql string, args ...interface{}) (Result, error)
	Query(sql string, args ...interface{}) (RowsInterface, error)
	QueryRow(sql string, args ...interface{}) RowInterface
	Migrate(name, sql string) error
}

var _ DBClient = (*DB)(nil)
var _ TxInterface = (*Tx)(nil)

func (db *DB) Exec(sql string, args ...interface{}) (Result, error) {
	qr, err := db.query(sql, args)
	if err != nil {
		return &execResult{rowsAffected: 0}, err
	}
	ra := int64(0)
	lid := int64(0)
	if len(qr.Rows) > 0 && len(qr.Columns) >= 2 {
		if v, ok := toInt64(qr.Rows[0][0]); ok {
			lid = v
		}
		if v, ok := toInt64(qr.Rows[0][len(qr.Rows[0])-1]); ok {
			ra = v
		}
	}
	return &execResult{rowsAffected: ra, lastInsertID: lid}, nil
}

func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	default:
		return 0, false
	}
}

func (db *DB) Query(sql string, args ...interface{}) (RowsInterface, error) {
	qr, err := db.query(sql, args)
	if err != nil {
		return nil, err
	}
	return &Rows{columns: qr.Columns, rows: qr.Rows, pos: 0}, nil
}

func (db *DB) QueryRow(sql string, args ...interface{}) RowInterface {
	qr, err := db.query(sql, args)
	if err != nil {
		return &Row{err: err}
	}
	if len(qr.Rows) == 0 {
		return &Row{err: fmt.Errorf("no rows")}
	}
	return &Row{columns: qr.Columns, row: qr.Rows[0]}
}

func (db *DB) Begin() (TxInterface, error) {
	return &Tx{db: db}, nil
}

func (db *DB) Migrate(name, sql string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.query("CREATE TABLE IF NOT EXISTS _migrations (id INTEGER PRIMARY KEY, name TEXT UNIQUE, applied_at TEXT)", nil)
	if err != nil {
		return fmt.Errorf("sparkdb: init migrations: %w", err)
	}

	qr, err := db.query("SELECT COUNT(*) FROM _migrations WHERE name = ?", []interface{}{name})
	if err != nil {
		return fmt.Errorf("sparkdb: check migration: %w", err)
	}
	if len(qr.Rows) > 0 && len(qr.Rows[0]) > 0 {
		if count, ok := qr.Rows[0][0].(float64); ok && count > 0 {
			return nil
		}
	}

	slog.Info("running migration", "name", name)
	statements := strings.Split(sql, ";")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("sparkdb: migration %s: %w", name, err)
		}
	}
	if _, err := tx.Exec("INSERT INTO _migrations (name, applied_at) VALUES (?, datetime('now'))", name); err != nil {
		tx.Rollback()
		return fmt.Errorf("sparkdb: record migration: %w", err)
	}
	return tx.Commit()
}

func (db *DB) Stats() map[string]any {
	return map[string]any{
		"server":   db.baseURL,
		"database": db.database,
		"authed":   db.token != "",
	}
}

func (db *DB) Close() error {
	slog.Info("closing SparkDB connection")
	return nil
}
