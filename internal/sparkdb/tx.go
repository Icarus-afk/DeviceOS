package sparkdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type Tx struct {
	db      *DB
	queries []string
	done    bool
}

func (tx *Tx) Exec(sql string, args ...interface{}) (Result, error) {
	if tx.done {
		return nil, fmt.Errorf("transaction already committed or rolled back")
	}
	// Build the full SQL with params inline since SparkDB transaction doesn't support params
	q := buildInlineSQL(sql, args)
	tx.queries = append(tx.queries, q)
	return &execResult{}, nil
}

func (tx *Tx) Commit() error {
	if tx.done {
		return fmt.Errorf("transaction already committed or rolled back")
	}
	tx.done = true
	if len(tx.queries) == 0 {
		return nil
	}

	body, _ := json.Marshal(map[string]interface{}{
		"database": tx.db.database,
		"queries":  tx.queries,
	})
	resp, err := tx.db.request("POST", "/transaction", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sparkdb: tx: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Error string `json:"error"`
	}
	raw, _ := io.ReadAll(resp.Body)
	json.Unmarshal(raw, &result)
	if result.Error != "" {
		return fmt.Errorf("sparkdb: tx: %s", result.Error)
	}
	return nil
}

func (tx *Tx) Rollback() error {
	if tx.done {
		return nil
	}
	tx.done = true
	return nil
}

func buildInlineSQL(sql string, args []interface{}) string {
	if len(args) == 0 {
		return sql
	}
	result := ""
	argIdx := 0
	for _, ch := range sql {
		if ch == '?' && argIdx < len(args) {
			result += fmt.Sprintf("'%v'", args[argIdx])
			argIdx++
		} else {
			result += string(ch)
		}
	}
	return result
}
