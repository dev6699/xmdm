package admin

import (
	"errors"
	"sync"
	"time"
)

type Record struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenantId"`
	Name      string         `json:"name"`
	Status    string         `json:"status"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

type Store struct {
	mu      sync.Mutex
	now     func() time.Time
	seq     map[string]int
	records map[string]map[string]Record
}

var ErrNotFound = errors.New("not found")

func NewStore() *Store {
	return &Store{
		now:     time.Now,
		seq:     make(map[string]int),
		records: make(map[string]map[string]Record),
	}
}

func (s *Store) SetNow(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

func (s *Store) Create(kind, tenantID, name string, extra map[string]any) Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq[kind]++
	if s.records[kind] == nil {
		s.records[kind] = make(map[string]Record)
	}
	rec := Record{
		ID:        kind + "-" + itoa(s.seq[kind]),
		TenantID:  tenantID,
		Name:      name,
		Status:    "active",
		UpdatedAt: s.now(),
		Extra:     cloneMap(extra),
	}
	s.records[kind][rec.ID] = rec
	return rec
}

func (s *Store) List(kind, tenantID string) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Record, 0)
	for _, rec := range s.records[kind] {
		if rec.TenantID != tenantID {
			continue
		}
		items = append(items, rec)
	}
	return items
}

func (s *Store) Update(kind, tenantID, id, name string, extra map[string]any) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[kind][id]
	if !ok || rec.TenantID != tenantID {
		return Record{}, ErrNotFound
	}
	if name != "" {
		rec.Name = name
	}
	if extra != nil {
		rec.Extra = cloneMap(extra)
	}
	rec.UpdatedAt = s.now()
	s.records[kind][id] = rec
	return rec, nil
}

func (s *Store) Retire(kind, tenantID, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[kind][id]
	if !ok || rec.TenantID != tenantID {
		return Record{}, ErrNotFound
	}
	now := s.now()
	rec.Status = "retired"
	rec.DeletedAt = &now
	rec.UpdatedAt = now
	s.records[kind][id] = rec
	return rec, nil
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
