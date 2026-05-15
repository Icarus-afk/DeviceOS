package sparkdb_test

import (
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
)

func TestNotFound(t *testing.T) {
	err := sparkdb.NotFound("device", "dev_123")
	if err.Error() != "device not found: dev_123" {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}

func TestIsNotFound(t *testing.T) {
	err := sparkdb.NotFound("device", "dev_123")
	if !sparkdb.IsNotFound(err) {
		t.Fatal("expected IsNotFound to be true")
	}
}

func TestIsNotFoundOnOther(t *testing.T) {
	if sparkdb.IsNotFound(nil) {
		t.Fatal("nil should not be not-found")
	}
}
