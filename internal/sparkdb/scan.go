package sparkdb

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func scanAssign(dest interface{}, src interface{}) error {
	if src == nil {
		return nil
	}

	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("scan: destination must be a non-nil pointer")
	}

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
		// Handle sql.NullString, sql.NullInt64, etc.
		sv := reflect.ValueOf(dest).Elem()
		if sv.Kind() == reflect.Struct {
			return scanStruct(sv, src)
		}
		return fmt.Errorf("scan: unsupported type %T", dest)
	}
	return nil
}

func scanStruct(sv reflect.Value, src interface{}) error {
	// Try to find a String/Valid pattern (sql.NullString, etc.)
	strField := sv.FieldByName("String")
	validField := sv.FieldByName("Valid")
	intField := sv.FieldByName("Int64")
	floatField := sv.FieldByName("Float64")
	timeField := sv.FieldByName("Time")

	if strField.IsValid() && validField.IsValid() && strField.Kind() == reflect.String {
		strField.SetString(fmt.Sprintf("%v", src))
		validField.SetBool(true)
		return nil
	}
	if intField.IsValid() && validField.IsValid() && intField.Kind() == reflect.Int64 {
		intField.SetInt(int64(toInt(src)))
		validField.SetBool(true)
		return nil
	}
	if floatField.IsValid() && validField.IsValid() && floatField.Kind() == reflect.Float64 {
		floatField.SetFloat(toFloat(src))
		validField.SetBool(true)
		return nil
	}
	if timeField.IsValid() && validField.IsValid() && timeField.Kind() == reflect.Struct {
		timeField.Set(reflect.ValueOf(toTime(src)))
		validField.SetBool(true)
		return nil
	}
	return fmt.Errorf("scan: unsupported struct type %s", sv.Type().Name())
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
		n, _ := strconv.Atoi(x)
		return n
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
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
		return x == "1" || strings.ToLower(x) == "true"
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
		t, err = time.Parse("2006-01-02T15:04:05Z07:00", x)
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
