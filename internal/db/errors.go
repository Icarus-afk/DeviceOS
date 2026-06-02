package db

import "fmt"

type ErrNotFound struct {
	Kind string
	ID   string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Kind, e.ID)
}

func IsNotFound(err error) bool {
	_, ok := err.(*ErrNotFound)
	return ok
}

func NotFound(kind, id string) error {
	return &ErrNotFound{Kind: kind, ID: id}
}
