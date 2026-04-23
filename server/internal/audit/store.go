package audit

import (
	"sync"
	"time"
)

type Event struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenantId"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	CreatedAt    time.Time      `json:"createdAt"`
	Details      map[string]any `json:"details,omitempty"`
}

type Store struct {
	mu     sync.Mutex
	now    func() time.Time
	seq    int
	events []Event
}

func NewStore() *Store {
	return &Store{
		now: time.Now,
	}
}

func (s *Store) SetNow(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

func (s *Store) Record(tenantID, actor, action, resourceType, resourceID string, details map[string]any) Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	event := Event{
		ID:           itoa(s.seq),
		TenantID:     tenantID,
		Actor:        actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		CreatedAt:    s.now(),
		Details:      cloneMap(details),
	}
	s.events = append(s.events, event)
	return event
}

func (s *Store) List(tenantID string) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]Event, 0, len(s.events))
	for _, event := range s.events {
		if event.TenantID != tenantID {
			continue
		}
		items = append(items, event)
	}
	return items
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
