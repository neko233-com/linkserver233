package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/linkserver233/internal/link"
)

func newRecord(path string, now time.Time) link.Record {
	return link.Record{
		Path:           path,
		TargetURL:      "https://example.com/docs",
		Description:    "docs",
		RedirectStatus: 302,
		Enabled:        true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func TestFileStorePersistsRecords(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "links.json")

	fs, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	record := newRecord("hello/world", now)

	if _, err := fs.Create(record); err != nil {
		t.Fatalf("create record: %v", err)
	}
	if _, err := fs.Create(record); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	if _, err := fs.RegisterVisit(record.Path); err != nil {
		t.Fatalf("register visit: %v", err)
	}

	reopened, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}

	loaded, err := reopened.Get(record.Path)
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if loaded.Clicks != 1 {
		t.Fatalf("expected 1 click, got %d", loaded.Clicks)
	}
	if loaded.LastVisitedAt == nil {
		t.Fatal("expected last_visited_at to be stored")
	}

	if err := reopened.Delete(record.Path); err != nil {
		t.Fatalf("delete record: %v", err)
	}
	if _, err := reopened.Get(record.Path); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRegisterVisitEnforcesLifecycle(t *testing.T) {
	now := time.Now().UTC()

	t.Run("disabled", func(t *testing.T) {
		fs := mustStore(t)
		record := newRecord("disabled", now)
		record.Enabled = false
		mustCreate(t, fs, record)

		if _, err := fs.RegisterVisit("disabled"); !errors.Is(err, ErrDisabled) {
			t.Fatalf("expected ErrDisabled, got %v", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		fs := mustStore(t)
		record := newRecord("expired", now)
		past := now.Add(-time.Hour)
		record.ExpiresAt = &past
		mustCreate(t, fs, record)

		if _, err := fs.RegisterVisit("expired"); !errors.Is(err, ErrExpired) {
			t.Fatalf("expected ErrExpired, got %v", err)
		}
	})

	t.Run("max clicks", func(t *testing.T) {
		fs := mustStore(t)
		record := newRecord("once", now)
		record.MaxClicks = 1
		mustCreate(t, fs, record)

		if _, err := fs.RegisterVisit("once"); err != nil {
			t.Fatalf("first visit: %v", err)
		}
		if _, err := fs.RegisterVisit("once"); !errors.Is(err, ErrExhausted) {
			t.Fatalf("expected ErrExhausted, got %v", err)
		}
	})
}

func TestReplaceAllAndDeleteExpired(t *testing.T) {
	now := time.Now().UTC()
	fs := mustStore(t)

	active := newRecord("active", now)
	expired := newRecord("expired", now)
	past := now.Add(-time.Minute)
	expired.ExpiresAt = &past

	if err := fs.ReplaceAll([]link.Record{active, expired}); err != nil {
		t.Fatalf("replace all: %v", err)
	}

	items, err := fs.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 links, got %d", len(items))
	}

	removed, err := fs.DeleteExpired(now)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	if _, err := fs.Get("expired"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expired link to be gone, got %v", err)
	}
	if _, err := fs.Get("active"); err != nil {
		t.Fatalf("expected active link to remain: %v", err)
	}
}

func mustStore(t *testing.T) *FileStore {
	t.Helper()
	fs, err := NewFileStore(filepath.Join(t.TempDir(), "links.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return fs
}

func mustCreate(t *testing.T, fs *FileStore, record link.Record) {
	t.Helper()
	if _, err := fs.Create(record); err != nil {
		t.Fatalf("create %q: %v", record.Path, err)
	}
}
