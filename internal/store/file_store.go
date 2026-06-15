package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/neko233-com/linkserver233/internal/link"
)

type FileStore struct {
	path  string
	mu    sync.RWMutex
	links map[string]link.Record
}

type filePayload struct {
	Links map[string]link.Record `json:"links"`
}

func NewFileStore(path string) (*FileStore, error) {
	store := &FileStore{
		path:  path,
		links: make(map[string]link.Record),
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *FileStore) Create(record link.Record) (link.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[record.Path]; exists {
		return link.Record{}, ErrConflict
	}

	s.links[record.Path] = record
	if err := s.persistLocked(); err != nil {
		delete(s.links, record.Path)
		return link.Record{}, err
	}

	return record, nil
}

func (s *FileStore) Update(record link.Record) (link.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[record.Path]; !exists {
		return link.Record{}, ErrNotFound
	}

	s.links[record.Path] = record
	if err := s.persistLocked(); err != nil {
		return link.Record{}, err
	}

	return record, nil
}

func (s *FileStore) Get(path string) (link.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.links[path]
	if !exists {
		return link.Record{}, ErrNotFound
	}

	return record, nil
}

func (s *FileStore) List() ([]link.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]link.Record, 0, len(s.links))
	for _, record := range s.links {
		items = append(items, record)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].Path < items[j].Path
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	return items, nil
}

func (s *FileStore) Delete(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[path]; !exists {
		return ErrNotFound
	}

	delete(s.links, path)
	if err := s.persistLocked(); err != nil {
		return err
	}

	return nil
}

func (s *FileStore) RegisterVisit(path string) (link.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.links[path]
	if !exists {
		return link.Record{}, ErrNotFound
	}

	now := time.Now().UTC()
	if !record.Enabled {
		return link.Record{}, ErrDisabled
	}
	if record.IsExpired(now) {
		return link.Record{}, ErrExpired
	}
	if record.IsExhausted() {
		return link.Record{}, ErrExhausted
	}

	record.Clicks++
	record.LastVisitedAt = &now
	s.links[path] = record

	if err := s.persistLocked(); err != nil {
		return link.Record{}, err
	}

	return record, nil
}

// ReplaceAll atomically swaps the entire link set, used for bulk imports.
func (s *FileStore) ReplaceAll(records []link.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	replacement := make(map[string]link.Record, len(records))
	for _, record := range records {
		replacement[record.Path] = record
	}

	previous := s.links
	s.links = replacement
	if err := s.persistLocked(); err != nil {
		s.links = previous
		return err
	}

	return nil
}

// DeleteExpired removes links whose expiration time has passed and returns the
// number of links that were purged.
func (s *FileStore) DeleteExpired(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := make([]string, 0)
	for path, record := range s.links {
		if record.IsExpired(now) {
			removed = append(removed, path)
		}
	}
	if len(removed) == 0 {
		return 0, nil
	}

	snapshot := make(map[string]link.Record, len(removed))
	for _, path := range removed {
		snapshot[path] = s.links[path]
		delete(s.links, path)
	}

	if err := s.persistLocked(); err != nil {
		for path, record := range snapshot {
			s.links[path] = record
		}
		return 0, err
	}

	return len(removed), nil
}

func (s *FileStore) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create store directory: %w", err)
	}

	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read store file: %w", err)
	}
	if len(content) == 0 {
		return nil
	}

	var payload filePayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return fmt.Errorf("decode store file: %w", err)
	}
	if payload.Links == nil {
		payload.Links = make(map[string]link.Record)
	}

	s.links = payload.Links
	return nil
}

func (s *FileStore) persistLocked() error {
	payload := filePayload{Links: s.links}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store file: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o644); err != nil {
		return fmt.Errorf("write temporary store file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("replace store file: %w", err)
		}
		if retryErr := os.Rename(tmpPath, s.path); retryErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("replace store file: %w", retryErr)
		}
	}

	return nil
}
