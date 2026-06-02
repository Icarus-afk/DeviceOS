package db

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
)

type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

type MigrationStatus struct {
	Version   int
	Name      string
	AppliedAt string
	Checksum  string
}

func (db *DB) Migrate(name, sql string) error {
	if err := db.ensureMigrationTable(); err != nil {
		return err
	}
	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE name = ?", name).Scan(&count)
	if err != nil {
		return fmt.Errorf("db: check migration: %w", err)
	}
	if count > 0 {
		return nil
	}

	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(sql)))
	slog.Info("running migration", "name", name)
	statements := strings.Split(sql, ";")
	tx, err := db.db.Begin()
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
			return fmt.Errorf("db: migration %s: %w", name, err)
		}
	}

	maxVer := 0
	db.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM _migrations").Scan(&maxVer)

	if _, err := tx.Exec("INSERT INTO _migrations (version, name, checksum) VALUES (?, ?, ?)", maxVer+1, name, checksum); err != nil {
		tx.Rollback()
		return fmt.Errorf("db: record migration: %w", err)
	}
	return tx.Commit()
}

type Migrator struct {
	db         *DB
	migrations []Migration
}

func (db *DB) NewMigrator() *Migrator {
	return &Migrator{db: db}
}

func (m *Migrator) Add(migrations ...Migration) {
	m.migrations = append(m.migrations, migrations...)
}

func (m *Migrator) Up() error {
	if err := m.db.ensureMigrationTable(); err != nil {
		return err
	}

	applied, err := m.db.appliedVersions()
	if err != nil {
		return err
	}

	for _, mig := range m.migrations {
		if applied[mig.Version] {
			continue
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(mig.Up)))
		slog.Info("applying migration", "version", mig.Version, "name", mig.Name)

		statements := strings.Split(mig.Up, ";")
		tx, err := m.db.db.Begin()
		if err != nil {
			return fmt.Errorf("db: begin tx for migration %d: %w", mig.Version, err)
		}
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("db: migration %d (%s): %w", mig.Version, mig.Name, err)
			}
		}
		if _, err := tx.Exec("INSERT OR REPLACE INTO _migrations (version, name, checksum) VALUES (?, ?, ?)",
			mig.Version, mig.Name, checksum); err != nil {
			tx.Rollback()
			return fmt.Errorf("db: record migration %d: %w", mig.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("db: commit migration %d: %w", mig.Version, err)
		}
	}
	return nil
}

func (m *Migrator) Down() error {
	if err := m.db.ensureMigrationTable(); err != nil {
		return err
	}

	var maxVersion int
	err := m.db.db.QueryRow("SELECT COALESCE(MAX(version), -1) FROM _migrations").Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("db: query max version: %w", err)
	}
	if maxVersion < 0 {
		return nil
	}

	for i := len(m.migrations) - 1; i >= 0; i-- {
		if m.migrations[i].Version == maxVersion {
			return m.revert(m.migrations[i])
		}
	}
	return nil
}

func (m *Migrator) DownTo(targetVersion int) error {
	if err := m.db.ensureMigrationTable(); err != nil {
		return err
	}

	applied, err := m.db.appliedVersions()
	if err != nil {
		return err
	}

	for i := len(m.migrations) - 1; i >= 0; i-- {
		mig := m.migrations[i]
		if !applied[mig.Version] {
			continue
		}
		if targetVersion >= 0 && mig.Version <= targetVersion {
			break
		}
		if err := m.revert(mig); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrator) revert(mig Migration) error {
	if mig.Down == "" {
		slog.Warn("no down migration for version", "version", mig.Version, "name", mig.Name)
		return nil
	}

	slog.Info("reverting migration", "version", mig.Version, "name", mig.Name)
	statements := strings.Split(mig.Down, ";")
	tx, err := m.db.db.Begin()
	if err != nil {
		return fmt.Errorf("db: begin tx for revert %d: %w", mig.Version, err)
	}
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("db: revert migration %d (%s): %w", mig.Version, mig.Name, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM _migrations WHERE version = ?", mig.Version); err != nil {
		tx.Rollback()
		return fmt.Errorf("db: unrecord migration %d: %w", mig.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit revert %d: %w", mig.Version, err)
	}
	return nil
}

func (m *Migrator) Status() ([]MigrationStatus, error) {
	if err := m.db.ensureMigrationTable(); err != nil {
		return nil, err
	}

	rows, err := m.db.db.Query("SELECT version, name, applied_at, checksum FROM _migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("db: query migrations: %w", err)
	}
	defer rows.Close()

	var statuses []MigrationStatus
	for rows.Next() {
		var s MigrationStatus
		if err := rows.Scan(&s.Version, &s.Name, &s.AppliedAt, &s.Checksum); err != nil {
			return nil, err
		}
		statuses = append(statuses, s)
	}
	return statuses, rows.Err()
}

func (db *DB) appliedVersions() (map[int]bool, error) {
	applied := make(map[int]bool)
	rows, err := db.db.Query("SELECT version FROM _migrations")
	if err != nil {
		return nil, fmt.Errorf("db: query applied versions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func (db *DB) ensureMigrationTable() error {
	_, err := db.db.Exec(`CREATE TABLE IF NOT EXISTS _migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT DEFAULT (datetime('now')),
		checksum TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("db: init migrations: %w", err)
	}

	var hasOldSchema int
	db.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('_migrations') WHERE name='id'`).Scan(&hasOldSchema)
	if hasOldSchema > 0 {
		slog.Info("upgrading _migrations table schema")
		tx, err := db.db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(`CREATE TABLE _migrations_new (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT DEFAULT (datetime('now')),
			checksum TEXT NOT NULL DEFAULT ''
		)`); err != nil {
			tx.Rollback()
			return err
		}

		if _, err := tx.Exec(`INSERT INTO _migrations_new (version, name, applied_at, checksum)
			SELECT 0, name, applied_at, '' FROM _migrations`); err != nil {
			tx.Rollback()
			return err
		}

		if _, err := tx.Exec("DROP TABLE _migrations"); err != nil {
			tx.Rollback()
			return err
		}

		if _, err := tx.Exec("ALTER TABLE _migrations_new RENAME TO _migrations"); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("db: migrate _migrations schema: %w", err)
		}

		// Re-create the unique index by name for backward compat lookups
		db.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_migrations_name ON _migrations(name)")
	} else {
		db.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_migrations_name ON _migrations(name)")
	}

	return nil
}
