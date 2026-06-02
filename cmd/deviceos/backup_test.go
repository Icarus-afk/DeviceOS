package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCreateSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	createTestDB(t, dbPath)

	snapshotPath, err := createSnapshot(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Dir(snapshotPath))

	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Fatal("snapshot file not created")
	}

	verifyDB(t, snapshotPath, 3)
}

func TestCompressAndDecompress(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.db")
	dest := filepath.Join(t.TempDir(), "backup.db.gz")
	createTestDB(t, src)

	_, err := compressFile(src, dest)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dest); os.IsNotExist(err) {
		t.Fatal("compressed file not created")
	}

	restoredPath, err := decompressAndVerify(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Dir(restoredPath))

	verifyDB(t, restoredPath, 3)
}

func TestCompressFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.db")
	dest := filepath.Join(t.TempDir(), "backup.db.gz")
	createTestDB(t, src)

	written, err := compressFile(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if written <= 0 {
		t.Fatal("expected bytes written > 0")
	}

	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() <= 0 {
		t.Fatal("expected output file size > 0")
	}
}

func TestDecompressAndVerify_InvalidFile(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "not-a-backup.gz")
	os.WriteFile(badPath, []byte("not actually gzip data"), 0644)

	_, err := decompressAndVerify(badPath)
	if err == nil {
		t.Fatal("expected error for invalid gzip")
	}
}

func TestDecompressAndVerify_NonExistent(t *testing.T) {
	_, err := decompressAndVerify("/nonexistent/file.gz")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestCreateSnapshot_NonExistent(t *testing.T) {
	_, err := createSnapshot("/nonexistent/db.sqlite")
	if err == nil {
		t.Fatal("expected error for nonexistent db")
	}
}

func TestFmtSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, c := range cases {
		got := fmtSize(c.bytes)
		if got != c.want {
			t.Errorf("fmtSize(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

func createTestDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS test_data (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 3; i++ {
		_, err = db.Exec("INSERT INTO test_data (id, val) VALUES (?, ?)", i, "value_"+string(rune('a'+i-1)))
		if err != nil {
			t.Fatal(err)
		}
	}
}

func verifyDB(t *testing.T, path string, expectedRows int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_data").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != expectedRows {
		t.Fatalf("expected %d rows, got %d", expectedRows, count)
	}
}
