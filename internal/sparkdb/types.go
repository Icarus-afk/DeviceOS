package sparkdb

import "fmt"

// Result is the interface for query execution results.
type Result interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

// RowsInterface defines methods for iterating over query results.
type RowsInterface interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close() error
	Err() error
}

// RowInterface defines methods for single-row query results.
type RowInterface interface {
	Scan(dest ...interface{}) error
}

var _ RowsInterface = (*Rows)(nil)
var _ RowInterface = (*Row)(nil)

type execResult struct {
	rowsAffected int64
	lastInsertID int64
}

func (r *execResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r *execResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type Rows struct {
	columns []string
	rows    [][]interface{}
	pos     int
}

func (r *Rows) Next() bool {
	if r.pos >= len(r.rows) {
		return false
	}
	r.pos++
	return true
}

func (r *Rows) Scan(dest ...interface{}) error {
	if r.pos == 0 || r.pos > len(r.rows) {
		return fmt.Errorf("scan: no row available")
	}
	row := r.rows[r.pos-1]
	for i, d := range dest {
		if i >= len(row) {
			continue
		}
		if err := scanAssign(d, row[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rows) Close() error { return nil }
func (r *Rows) Err() error   { return nil }
func (r *Rows) Columns() ([]string, error) { return r.columns, nil }

type Row struct {
	columns []string
	row     []interface{}
	err     error
}

func (r *Row) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if r.row == nil {
		return fmt.Errorf("no rows")
	}
	for i, d := range dest {
		if i >= len(r.row) {
			continue
		}
		if err := scanAssign(d, r.row[i]); err != nil {
			return err
		}
	}
	return nil
}
