package main

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lohtbrok/deviceos/internal/config"
)

func cmdBackup(output string) {
	cfg, err := config.Load("deviceos.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	src := cfg.Storage.Path
	if src == "" {
		fmt.Fprintln(os.Stderr, "error: storage.path not configured")
		os.Exit(1)
	}

	_, err = os.Stat(src)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: database not found at %s\n", src)
		os.Exit(1)
	}

	if output == "" {
		ts := time.Now().UTC().Format("2006-01-02T150405Z")
		output = fmt.Sprintf("backup-%s.db.gz", ts)
	}

	snapshotPath, err := createSnapshot(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(snapshotPath)

	written, err := compressFile(snapshotPath, output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fi, _ := os.Stat(output)
	fmt.Printf("Backup created: %s (%s, %d bytes)\n", output, fmtSize(fi.Size()), fi.Size())
	fmt.Printf("Database size:  %s (%d bytes)\n", fmtSize(written), written)
}

func cmdRestore(backupFile string) {
	if backupFile == "" {
		fmt.Fprintln(os.Stderr, "usage: deviceos restore <backup-file>")
		os.Exit(1)
	}

	_, err := os.Stat(backupFile)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: backup file not found: %s\n", backupFile)
		os.Exit(1)
	}

	cfg, err := config.Load("deviceos.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	dest := cfg.Storage.Path

	restoredPath, err := decompressAndVerify(backupFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(restoredPath)

	backupStat, _ := os.Stat(backupFile)
	fmt.Printf("Restoring from: %s (%s)\n", backupFile, fmtSize(backupStat.Size()))
	fmt.Printf("Destination:    %s\n", dest)
	fmt.Println()
	fmt.Println("WARNING: This will overwrite the current database.")
	fmt.Print("Continue? [y/N] ")

	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "y" && confirm != "Y" {
		fmt.Println("Restore cancelled.")
		os.Exit(0)
	}

	if _, err := os.Stat(dest); err == nil {
		backupPath := dest + ".bak"
		if err := os.Rename(dest, backupPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: backup current database: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Current database backed up to: %s\n", backupPath)
	}

	if err := os.Rename(restoredPath, dest); err != nil {
		fmt.Fprintf(os.Stderr, "error: restore database: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Restore complete.")
}

func createSnapshot(src string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "deviceos-snapshot-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	snapshotPath := filepath.Join(tmpDir, "snapshot.db")

	db, err := sql.Open("sqlite", src)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("open database: %w", err)
	}

	_, err = db.Exec(fmt.Sprintf("VACUUM INTO '%s'", snapshotPath))
	db.Close()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("create snapshot: %w", err)
	}

	return snapshotPath, nil
}

func compressFile(src, dest string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return 0, fmt.Errorf("create output: %w", err)
	}

	gz, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	if err != nil {
		out.Close()
		os.Remove(dest)
		return 0, fmt.Errorf("init gzip: %w", err)
	}

	written, err := io.Copy(gz, in)
	if err != nil {
		gz.Close()
		out.Close()
		os.Remove(dest)
		return 0, fmt.Errorf("compress: %w", err)
	}

	if err := gz.Close(); err != nil {
		out.Close()
		os.Remove(dest)
		return 0, fmt.Errorf("finalize gzip: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(dest)
		return 0, fmt.Errorf("close output: %w", err)
	}

	return written, nil
}

func decompressAndVerify(backupFile string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "deviceos-verify-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	restoredPath := filepath.Join(tmpDir, "restored.db")

	backup, err := os.Open(backupFile)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("open backup: %w", err)
	}
	defer backup.Close()

	gz, err := gzip.NewReader(backup)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("invalid gzip: %w", err)
	}
	defer gz.Close()

	out, err := os.Create(restoredPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("create temp file: %w", err)
	}

	_, err = io.Copy(out, gz)
	out.Close()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("decompress: %w", err)
	}

	db, err := sql.Open("sqlite", restoredPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("open restored: %w", err)
	}
	var integrity string
	err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity)
	db.Close()
	if err != nil || integrity != "ok" {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("integrity check failed: %s", integrity)
	}

	return restoredPath, nil
}

func fmtSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
