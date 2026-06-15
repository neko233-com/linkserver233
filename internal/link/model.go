package link

import "time"

// Status describes the runtime state of a link.
type Status string

const (
	StatusActive    Status = "active"
	StatusDisabled  Status = "disabled"
	StatusExpired   Status = "expired"
	StatusExhausted Status = "exhausted"
)

// Record is a stored link mapping plus its lifecycle metadata.
type Record struct {
	Path           string     `json:"path"`
	TargetURL      string     `json:"target_url"`
	Description    string     `json:"description,omitempty"`
	RedirectStatus int        `json:"redirect_status"`
	Tags           []string   `json:"tags,omitempty"`
	Enabled        bool       `json:"enabled"`
	PasswordHash   string     `json:"password_hash,omitempty"`
	MaxClicks      uint64     `json:"max_clicks,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Clicks         uint64     `json:"clicks"`
	LastVisitedAt  *time.Time `json:"last_visited_at,omitempty"`
}

// HasPassword reports whether the link is password protected.
func (r Record) HasPassword() bool {
	return r.PasswordHash != ""
}

// IsExpired reports whether the link has passed its expiration time.
func (r Record) IsExpired(now time.Time) bool {
	return r.ExpiresAt != nil && !now.Before(*r.ExpiresAt)
}

// IsExhausted reports whether the link reached its maximum click budget.
func (r Record) IsExhausted() bool {
	return r.MaxClicks > 0 && r.Clicks >= r.MaxClicks
}

// RemainingClicks returns how many visits remain, or nil when unlimited.
func (r Record) RemainingClicks() *uint64 {
	if r.MaxClicks == 0 {
		return nil
	}
	if r.Clicks >= r.MaxClicks {
		zero := uint64(0)
		return &zero
	}
	remaining := r.MaxClicks - r.Clicks
	return &remaining
}

// Status derives the link's runtime status at the given time.
func (r Record) Status(now time.Time) Status {
	switch {
	case !r.Enabled:
		return StatusDisabled
	case r.IsExpired(now):
		return StatusExpired
	case r.IsExhausted():
		return StatusExhausted
	default:
		return StatusActive
	}
}
