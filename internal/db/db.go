package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	Path         string
	MaxOpenConns int
	MaxIdleConns int
}

type DBClient interface {
	Exec(sql string, args ...interface{}) (Result, error)
	Query(sql string, args ...interface{}) (RowsInterface, error)
	QueryRow(sql string, args ...interface{}) RowInterface
	Migrate(name, sql string) error
}

type Result interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

type RowsInterface interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close() error
	Err() error
}

type RowInterface interface {
	Scan(dest ...interface{}) error
}

type TxInterface interface {
	Exec(sql string, args ...interface{}) (Result, error)
	Commit() error
	Rollback() error
}

type DB struct {
	db *sql.DB
}

type dbResult struct {
	sql.Result
}

func (r *dbResult) LastInsertId() (int64, error) { return r.Result.LastInsertId() }
func (r *dbResult) RowsAffected() (int64, error) { return r.Result.RowsAffected() }

type dbRows struct {
	*sql.Rows
}

func (r *dbRows) Scan(dest ...interface{}) error { return r.Rows.Scan(dest...) }
func (r *dbRows) Close() error                   { return r.Rows.Close() }
func (r *dbRows) Err() error                     { return r.Rows.Err() }

type dbRow struct {
	*sql.Row
}

func (r *dbRow) Scan(dest ...interface{}) error { return r.Row.Scan(dest...) }

type dbTx struct {
	tx *sql.Tx
}

func (t *dbTx) Exec(sql string, args ...interface{}) (Result, error) {
	res, err := t.tx.Exec(sql, args...)
	if err != nil {
		return nil, err
	}
	return &dbResult{Result: res}, nil
}

func (t *dbTx) Commit() error   { return t.tx.Commit() }
func (t *dbTx) Rollback() error { return t.tx.Rollback() }

func Open(cfg Config) (*DB, error) {
	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}

	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 25
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 5
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-8192",
		"PRAGMA temp_store=MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("db: %s: %w", p, err)
		}
	}

	slog.Info("sqlite database opened", "path", cfg.Path, "max_open", cfg.MaxOpenConns)
	return &DB{db: db}, nil
}

func (db *DB) Exec(sql string, args ...interface{}) (Result, error) {
	res, err := db.db.Exec(sql, args...)
	if err != nil {
		return nil, err
	}
	return &dbResult{Result: res}, nil
}

func (db *DB) Query(sql string, args ...interface{}) (RowsInterface, error) {
	rows, err := db.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	return &dbRows{Rows: rows}, nil
}

func (db *DB) QueryRow(sql string, args ...interface{}) RowInterface {
	return &dbRow{Row: db.db.QueryRow(sql, args...)}
}

func (db *DB) Begin() (TxInterface, error) {
	tx, err := db.db.Begin()
	if err != nil {
		return nil, err
	}
	return &dbTx{tx: tx}, nil
}

func (db *DB) Close() error {
	slog.Info("closing database")
	return db.db.Close()
}

var _ DBClient = (*DB)(nil)
var _ TxInterface = (*dbTx)(nil)
