package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateOldAPI(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "oldapi.db"))

	err := db.Migrate("test_v1", "CREATE TABLE IF NOT EXISTS test_old (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	err = db.Migrate("test_v1", "CREATE TABLE IF NOT EXISTS test_old (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	var name string
	err = db.db.QueryRow("SELECT name FROM _migrations WHERE name = ?", "test_v1").Scan(&name)
	if err != nil {
		t.Fatal("migration not recorded:", err)
	}
	if name != "test_v1" {
		t.Fatalf("expected test_v1, got %s", name)
	}
}

func TestMigratorUp(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "up.db"))

	m := db.NewMigrator()
	m.Add(
		Migration{Version: 1, Name: "create_users", Up: "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)"},
		Migration{Version: 2, Name: "create_posts", Up: "CREATE TABLE IF NOT EXISTS posts (id INTEGER PRIMARY KEY, title TEXT, user_id INTEGER)"},
	)

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}

	tables := listTables(t, db)
	if !contains(tables, "users") {
		t.Fatal("users table not created")
	}
	if !contains(tables, "posts") {
		t.Fatal("posts table not created")
	}

	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(status))
	}
}

func TestMigratorIdempotent(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "idempotent.db"))

	m := db.NewMigrator()
	m.Add(
		Migration{Version: 1, Name: "create_items", Up: "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT)"},
	)

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}

	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 {
		t.Fatalf("expected 1 migration after second Up, got %d", len(status))
	}
}

func TestMigratorDown(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "down.db"))

	m := db.NewMigrator()
	m.Add(
		Migration{Version: 1, Name: "create_a", Up: "CREATE TABLE IF NOT EXISTS table_a (id INTEGER PRIMARY KEY)", Down: "DROP TABLE IF EXISTS table_a"},
		Migration{Version: 2, Name: "create_b", Up: "CREATE TABLE IF NOT EXISTS table_b (id INTEGER PRIMARY KEY)", Down: "DROP TABLE IF EXISTS table_b"},
	)

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}

	if err := m.Down(); err != nil {
		t.Fatal(err)
	}

	tables := listTables(t, db)
	if contains(tables, "table_b") {
		t.Fatal("table_b should have been dropped")
	}
	if !contains(tables, "table_a") {
		t.Fatal("table_a should still exist")
	}

	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 {
		t.Fatalf("expected 1 migration after down, got %d", len(status))
	}
}

func TestMigratorDownTo(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "downto.db"))

	m := db.NewMigrator()
	m.Add(
		Migration{Version: 1, Name: "v1", Up: "CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY)", Down: "DROP TABLE IF EXISTS t1"},
		Migration{Version: 2, Name: "v2", Up: "CREATE TABLE IF NOT EXISTS t2 (id INTEGER PRIMARY KEY)", Down: "DROP TABLE IF EXISTS t2"},
		Migration{Version: 3, Name: "v3", Up: "CREATE TABLE IF NOT EXISTS t3 (id INTEGER PRIMARY KEY)", Down: "DROP TABLE IF EXISTS t3"},
	)

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}

	if err := m.DownTo(1); err != nil {
		t.Fatal(err)
	}

	tables := listTables(t, db)
	if contains(tables, "t3") {
		t.Fatal("t3 should have been dropped")
	}
	if contains(tables, "t2") {
		t.Fatal("t2 should have been dropped")
	}
	if !contains(tables, "t1") {
		t.Fatal("t1 should still exist")
	}

	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(status))
	}
}

func TestMigratorDownToZero(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "downto0.db"))

	m := db.NewMigrator()
	m.Add(
		Migration{Version: 1, Name: "v1", Up: "CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY)", Down: "DROP TABLE IF EXISTS t1"},
	)

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}

	if err := m.DownTo(0); err != nil {
		t.Fatal(err)
	}

	tables := listTables(t, db)
	if contains(tables, "t1") {
		t.Fatal("t1 should have been dropped")
	}

	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 0 {
		t.Fatalf("expected 0 migrations, got %d", len(status))
	}
}

func TestMigratorStatus(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "status.db"))

	m := db.NewMigrator()
	m.Add(
		Migration{Version: 1, Name: "v1", Up: "CREATE TABLE IF NOT EXISTS s1 (id INTEGER PRIMARY KEY)"},
	)

	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 0 {
		t.Fatalf("expected empty status, got %d entries", len(status))
	}

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}

	status, err = m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(status))
	}
	if status[0].Version != 1 {
		t.Fatalf("expected version 1, got %d", status[0].Version)
	}
	if status[0].Name != "v1" {
		t.Fatalf("expected name v1, got %s", status[0].Name)
	}
	if status[0].Checksum == "" {
		t.Fatal("expected non-empty checksum")
	}
}

func TestMigrateOldAPICompat(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "compat.db"))

	err := db.Migrate("legacy_devices", "CREATE TABLE IF NOT EXISTS legacy_devices (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	m := db.NewMigrator()
	m.Add(
		Migration{Version: 100, Name: "add_index", Up: "CREATE INDEX IF NOT EXISTS idx_legacy_name ON legacy_devices(name)"},
	)

	if err := m.Up(); err != nil {
		t.Fatal(err)
	}

	status, err := m.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 2 {
		t.Fatalf("expected 2 migrations (1 legacy + 1 versioned), got %d", len(status))
	}
}

func TestLegacySchemaUpgrade(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "legacy_schema.db"))

	_, err := db.db.Exec(`CREATE TABLE IF NOT EXISTS _migrations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE,
		applied_at TEXT DEFAULT (datetime('now'))
	)`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.db.Exec("INSERT INTO _migrations (name) VALUES ('old_migration_v1')")
	if err != nil {
		t.Fatal(err)
	}

	err = db.Migrate("new_migration", "CREATE TABLE IF NOT EXISTS new_table (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}

	var version int
	err = db.db.QueryRow("SELECT version FROM _migrations WHERE name = ?", "old_migration_v1").Scan(&version)
	if err != nil {
		t.Fatal("old migration not preserved:", err)
	}
	if version != 0 {
		t.Fatalf("expected version 0 for legacy migration, got %d", version)
	}

	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM _migrations").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 total migrations, got %d", count)
	}
}

func openTestDB(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(path)
	})
	return db
}

func listTables(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE '_migrations%' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	return tables
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
