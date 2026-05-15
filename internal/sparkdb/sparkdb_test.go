package sparkdb_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
)

type mockSparkDB struct {
	migrations []string
}

func (m *mockSparkDB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/auth/login":
		json.NewEncoder(w).Encode(map[string]string{"token": "mock-token"})
	case "/auth/api-key":
		json.NewEncoder(w).Encode(map[string]string{"token": "mock-token"})
	case "/query":
		m.handleQuery(w, r)
	case "/backup":
		json.NewEncoder(w).Encode(map[string]string{"message": "ok", "file": "/tmp/backup.db"})
	case "/backups":
		json.NewEncoder(w).Encode(map[string]any{"backups": []string{}})
	case "/transaction":
		m.handleTransaction(w, r)
	default:
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}
}

func (m *mockSparkDB) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query    string        `json:"query"`
		Database string        `json:"database"`
		Params   []interface{} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "bad request"})
		return
	}

	upper := strings.ToUpper(strings.TrimSpace(req.Query))

	if strings.Contains(upper, "_MIGRATION") {
		m.handleMigrationQuery(w, upper, req.Params)
		return
	}

	if strings.HasPrefix(upper, "INSERT") {
		json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{},
			"rows":    [][]interface{}{},
		})
		return
	}

	if strings.Contains(upper, "SELECT COUNT(*)") {
		json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"COUNT(*)"},
			"rows":    [][]interface{}{{1.0}},
		})
		return
	}

	if strings.Contains(upper, "NAME, VALUE") {
		json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"name", "value"},
			"rows":    [][]interface{}{{"item1", 10.5}},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"columns": []string{},
		"rows":    [][]interface{}{},
	})
}

func (m *mockSparkDB) handleMigrationQuery(w http.ResponseWriter, upper string, params []interface{}) {
	if strings.Contains(upper, "CREATE TABLE") {
		json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{},
			"rows":    [][]interface{}{},
		})
		return
	}

	if strings.Contains(upper, "SELECT COUNT") && len(params) > 0 {
		name := params[0].(string)
		count := 0.0
		for _, mig := range m.migrations {
			if mig == name {
				count = 1.0
				break
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"COUNT(*)"},
			"rows":    [][]interface{}{{count}},
		})
		return
	}

	if strings.Contains(upper, "SELECT NAME") {
		rows := make([][]interface{}, len(m.migrations))
		for i, mig := range m.migrations {
			rows[i] = []interface{}{mig}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"name"},
			"rows":    rows,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"columns": []string{},
		"rows":    [][]interface{}{},
	})
}

func (m *mockSparkDB) handleTransaction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database string   `json:"database"`
		Queries  []string `json:"queries"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	for _, q := range req.Queries {
		if name := extractMigrationName(q); name != "" {
			m.migrations = append(m.migrations, name)
		}
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
}

func extractMigrationName(query string) string {
	upper := strings.ToUpper(query)
	if !strings.Contains(upper, "INSERT INTO _MIGRATIONS") {
		return ""
	}
	valsIdx := strings.Index(upper, "VALUES")
	if valsIdx < 0 {
		return ""
	}
	rest := query[valsIdx+6:]
	quote1 := strings.Index(rest, "'")
	if quote1 < 0 {
		return ""
	}
	rest = rest[quote1+1:]
	quote2 := strings.Index(rest, "'")
	if quote2 < 0 {
		return ""
	}
	return rest[:quote2]
}

func sparkdbConfig(srv *httptest.Server) sparkdb.Config {
	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	return sparkdb.Config{
		Host:     host,
		Port:     port,
		Database: "testdb",
	}
}

func TestOpenClose(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestMigrate(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate("test_v1", "CREATE TABLE IF NOT EXISTS test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := db.Migrate("test_v1", "CREATE TABLE IF NOT EXISTS test (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("second migrate should be noop: %v", err)
	}

	if err := db.Migrate("test_v2", "CREATE TABLE IF NOT EXISTS test2 (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("migrate v2: %v", err)
	}
}

func TestExecAndQuery(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Migrate("test", "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, value REAL)")

	_, err = db.Exec("INSERT INTO items (name, value) VALUES (?, ?)", "item1", 10.5)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM items").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	var name string
	var value float64
	err = db.QueryRow("SELECT name, value FROM items WHERE id = ?", 1).Scan(&name, &value)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if name != "item1" || value != 10.5 {
		t.Fatalf("expected (item1, 10.5), got (%s, %f)", name, value)
	}
}

func TestBackup(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	path, err := db.Backup()
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty backup path")
	}
}

func TestListBackupsEmpty(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	backups, err := db.ListBackups()
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected 0 backups, got %d", len(backups))
	}
}

func TestStats(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats["database"] != "testdb" {
		t.Fatalf("expected database=testdb, got %v", stats["database"])
	}
}

func TestDoubleClose(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal("double close should not error")
	}
}

func TestOpenFailure(t *testing.T) {
	_, err := sparkdb.Open(sparkdb.Config{Host: "127.0.0.1", Port: 1})
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestMultipleMigrations(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migs := []struct {
		name string
		sql  string
	}{
		{"v1", "CREATE TABLE t1 (id INTEGER PRIMARY KEY)"},
		{"v2", "CREATE TABLE t2 (id INTEGER PRIMARY KEY)"},
		{"v3", "CREATE TABLE t3 (id INTEGER PRIMARY KEY)"},
	}
	for _, m := range migs {
		if err := db.Migrate(m.name, m.sql); err != nil {
			t.Fatalf("migrate %s: %v", m.name, err)
		}
	}

	rows, err := db.Query("SELECT name FROM _migrations ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(names))
	}
}

func TestBeginCommit(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestBeginRollback(t *testing.T) {
	srv := httptest.NewServer(&mockSparkDB{})
	defer srv.Close()

	db, err := sparkdb.Open(sparkdbConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}
