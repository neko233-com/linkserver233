package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/neko233-com/linkserver233/internal/link"
	"github.com/neko233-com/linkserver233/internal/store"
)

type importLink struct {
	Path           string     `json:"path"`
	TargetURL      string     `json:"target_url"`
	Description    string     `json:"description"`
	RedirectStatus int        `json:"redirect_status"`
	Tags           []string   `json:"tags"`
	Password       string     `json:"password"`
	PasswordHash   string     `json:"password_hash"`
	MaxClicks      uint64     `json:"max_clicks"`
	ExpiresIn      string     `json:"expires_in"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Enabled        *bool      `json:"enabled"`
}

type importRequest struct {
	Replace bool         `json:"replace"`
	Links   []importLink `json:"links"`
}

type importResponse struct {
	Imported int      `json:"imported"`
	Replaced bool     `json:"replaced"`
	Skipped  []string `json:"skipped,omitempty"`
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	var request importRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(request.Links) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("links is required"))
		return
	}

	now := s.now().UTC()
	records := make([]link.Record, 0, len(request.Links))
	seen := make(map[string]struct{}, len(request.Links))

	for index, item := range request.Links {
		record, err := s.buildImportRecord(now, item)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("links[%d]: %w", index, err))
			return
		}
		if _, exists := seen[record.Path]; exists {
			writeError(w, http.StatusBadRequest, fmt.Errorf("links[%d]: duplicate path %q", index, record.Path))
			return
		}
		seen[record.Path] = struct{}{}
		records = append(records, record)
	}

	if request.Replace {
		if err := s.store.ReplaceAll(records); err != nil {
			s.logger.Error("import replace failed", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("failed to import links"))
			return
		}
		writeJSON(w, http.StatusOK, importResponse{Imported: len(records), Replaced: true})
		return
	}

	imported := 0
	skipped := make([]string, 0)
	for _, record := range records {
		if _, err := s.store.Create(record); err != nil {
			if errors.Is(err, store.ErrConflict) {
				skipped = append(skipped, record.Path)
				continue
			}
			s.logger.Error("import create failed", "path", record.Path, "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("failed to import links"))
			return
		}
		imported++
	}

	writeJSON(w, http.StatusOK, importResponse{Imported: imported, Skipped: skipped})
}

func (s *Server) buildImportRecord(now time.Time, item importLink) (link.Record, error) {
	pathValue, err := link.NormalizePath(item.Path)
	if err != nil {
		return link.Record{}, err
	}

	targetURL, err := link.NormalizeTargetURL(item.TargetURL, s.cfg.AllowPrivateTargets)
	if err != nil {
		return link.Record{}, err
	}

	redirectStatus, err := link.NormalizeRedirectStatus(item.RedirectStatus)
	if err != nil {
		return link.Record{}, err
	}

	expiresAt, err := s.resolveExpiry(now, item.ExpiresAt, item.ExpiresIn, nil, false, false)
	if err != nil {
		return link.Record{}, err
	}

	passwordHash := strings.TrimSpace(item.PasswordHash)
	if passwordHash == "" && item.Password != "" {
		passwordHash, err = hashOptionalPassword(item.Password)
		if err != nil {
			return link.Record{}, err
		}
	}

	enabled := true
	if item.Enabled != nil {
		enabled = *item.Enabled
	}

	return link.Record{
		Path:           pathValue,
		TargetURL:      targetURL,
		Description:    strings.TrimSpace(item.Description),
		RedirectStatus: redirectStatus,
		Tags:           normalizeTags(item.Tags),
		Enabled:        enabled,
		PasswordHash:   passwordHash,
		MaxClicks:      item.MaxClicks,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}
