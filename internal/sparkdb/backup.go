package sparkdb

import (
	"encoding/json"
	"fmt"
	"io"
)

func (db *DB) Backup() (string, error) {
	resp, err := db.request("POST", "/backup", nil)
	if err != nil {
		return "", fmt.Errorf("sparkdb: backup: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		File    string `json:"file"`
	}
	json.Unmarshal(raw, &result)
	if result.Error != "" {
		return "", fmt.Errorf("sparkdb: backup: %s", result.Error)
	}
	return result.File, nil
}

func (db *DB) ListBackups() ([]string, error) {
	resp, err := db.request("GET", "/backups", nil)
	if err != nil {
		return nil, fmt.Errorf("sparkdb: list backups: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result struct {
		Backups []interface{} `json:"backups"`
		Error   string        `json:"error"`
	}
	json.Unmarshal(raw, &result)
	if result.Error != "" {
		return nil, fmt.Errorf("sparkdb: list backups: %s", result.Error)
	}
	var files []string
	for _, b := range result.Backups {
		if s, ok := b.(string); ok {
			files = append(files, s)
		}
	}
	return files, nil
}
