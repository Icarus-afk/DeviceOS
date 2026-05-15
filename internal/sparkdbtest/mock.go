package sparkdbtest

import (
	"fmt"
	"reflect"
	"time"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
)

// MockDB implements sparkdb.DBClient for testing.
type MockDB struct {
	OnExec    func(sql string, args []interface{}) (sparkdb.Result, error)
	OnQuery   func(sql string, args []interface{}) (sparkdb.RowsInterface, error)
	OnQueryRow func(sql string, args []interface{}) sparkdb.RowInterface
	OnMigrate func(name, sql string) error
}

func (m *MockDB) Exec(sql string, args ...interface{}) (sparkdb.Result, error) {
	if m.OnExec != nil {
		return m.OnExec(sql, args)
	}
	return &MockResult{}, nil
}

func (m *MockDB) Query(sql string, args ...interface{}) (sparkdb.RowsInterface, error) {
	if m.OnQuery != nil {
		return m.OnQuery(sql, args)
	}
	return &MockRows{}, nil
}

func (m *MockDB) QueryRow(sql string, args ...interface{}) sparkdb.RowInterface {
	if m.OnQueryRow != nil {
		return m.OnQueryRow(sql, args)
	}
	return &MockRow{Err: fmt.Errorf("no rows")}
}

func (m *MockDB) Migrate(name, sql string) error {
	if m.OnMigrate != nil {
		return m.OnMigrate(name, sql)
	}
	return nil
}

// MockResult implements sparkdb.Result.
type MockResult struct {
	LastID  int64
	Affected int64
}

func (r *MockResult) LastInsertId() (int64, error) { return r.LastID, nil }
func (r *MockResult) RowsAffected() (int64, error) { return r.Affected, nil }

// MockRows implements sparkdb.RowsInterface for testing.
type MockRows struct {
	Columns []string
	Rows    [][]interface{}
	Pos     int
	ErrVal  error
}

func (r *MockRows) Next() bool {
	if r.Pos >= len(r.Rows) {
		return false
	}
	r.Pos++
	return true
}

func (r *MockRows) Scan(dest ...interface{}) error {
	if r.ErrVal != nil {
		return r.ErrVal
	}
	if r.Pos == 0 || r.Pos > len(r.Rows) {
		return fmt.Errorf("scan: no row available")
	}
	row := r.Rows[r.Pos-1]
	if err := assignScan(dest, row); err != nil {
		return err
	}
	return nil
}

func (r *MockRows) Close() error { return nil }
func (r *MockRows) Err() error   { return r.ErrVal }

// MockRow implements sparkdb.RowInterface for testing.
type MockRow struct {
	Row  []interface{}
	Err  error
}

func (r *MockRow) Scan(dest ...interface{}) error {
	if r.Err != nil {
		return r.Err
	}
	if len(r.Row) == 0 {
		return fmt.Errorf("no rows")
	}
	return assignScan(dest, r.Row)
}

func assignScan(dest []interface{}, src []interface{}) error {
	for i, d := range dest {
		if i >= len(src) {
			break
		}
		val := src[i]
		if val == nil {
			continue
		}
		if err := setValue(d, val); err != nil {
			return err
		}
	}
	return nil
}

func setValue(dest interface{}, src interface{}) error {
	switch d := dest.(type) {
	case *string:
		*d = fmt.Sprintf("%v", src)
	case *int:
		*d = toInt(src)
	case *int64:
		*d = int64(toInt(src))
	case *float64:
		*d = toFloat(src)
	case *bool:
		*d = toBool(src)
	case *time.Time:
		*d = toTime(src)
	case *[]byte:
		*d = []byte(fmt.Sprintf("%v", src))
	case *interface{}:
		*d = src
	default:
		rv := reflect.ValueOf(dest)
		if rv.Kind() == reflect.Ptr {
			ev := rv.Elem()
			if ev.Kind() == reflect.Struct {
				if sf := ev.FieldByName("String"); sf.IsValid() && sf.Kind() == reflect.String {
					if vf := ev.FieldByName("Valid"); vf.IsValid() && vf.Kind() == reflect.Bool {
						sf.SetString(fmt.Sprintf("%v", src))
						vf.SetBool(true)
						return nil
					}
				}
				if tf := ev.FieldByName("Time"); tf.IsValid() && tf.Kind() == reflect.Struct {
					if vf := ev.FieldByName("Valid"); vf.IsValid() && vf.Kind() == reflect.Bool {
						tf.Set(reflect.ValueOf(toTime(src)))
						vf.SetBool(true)
						return nil
					}
				}
				if nf := ev.FieldByName("Int64"); nf.IsValid() && nf.Kind() == reflect.Int64 {
					if vf := ev.FieldByName("Valid"); vf.IsValid() && vf.Kind() == reflect.Bool {
						nf.SetInt(int64(toInt(src)))
						vf.SetBool(true)
						return nil
					}
				}
			}
		}
		return fmt.Errorf("scan: unsupported type %T", dest)
	}
	return nil
}

func toInt(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int64:
		return int(x)
	case int:
		return x
	case string:
		n := 0
		fmt.Sscanf(x, "%d", &n)
		return n
	default:
		return 0
	}
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case string:
		f := 0.0
		fmt.Sscanf(x, "%f", &f)
		return f
	default:
		return 0
	}
}

func toBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x == "1" || x == "true"
	default:
		return false
	}
}

func toTime(v interface{}) time.Time {
	switch x := v.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, x)
		if err == nil {
			return t
		}
		t, err = time.Parse("2006-01-02 15:04:05", x)
		if err == nil {
			return t
		}
	case time.Time:
		return x
	}
	return time.Time{}
}
