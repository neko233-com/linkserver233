package store

import (
	"errors"
	"time"

	"github.com/neko233-com/linkserver233/internal/link"
)

var (
	ErrNotFound  = errors.New("link not found")
	ErrConflict  = errors.New("link already exists")
	ErrDisabled  = errors.New("link is disabled")
	ErrExpired   = errors.New("link has expired")
	ErrExhausted = errors.New("link click limit reached")
)

// Store persists link records and enforces visit lifecycle atomically.
type Store interface {
	Create(record link.Record) (link.Record, error)
	Update(record link.Record) (link.Record, error)
	Get(path string) (link.Record, error)
	List() ([]link.Record, error)
	Delete(path string) error
	RegisterVisit(path string) (link.Record, error)
	ReplaceAll(records []link.Record) error
	DeleteExpired(now time.Time) (int, error)
}
