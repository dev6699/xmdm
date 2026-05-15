package auth

import (
	"crypto/subtle"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

const SessionCookieName = "xmdm_session"

var ErrInvalidCredentials = errors.New("invalid credentials")

type Session struct {
	ID          string
	Username    string
	Permissions []Permission
	ExpiresAt   time.Time
}

type Service struct {
	username    string
	password    string
	sessionTTL  time.Duration
	permissions []Permission

	mu       sync.Mutex
	sessions map[string]Session
	now      func() time.Time
}

func NewService(username, password string, sessionTTL time.Duration) *Service {
	return NewServiceWithPermissions(username, password, sessionTTL, AllPermissions())
}

func NewServiceWithPermissions(username, password string, sessionTTL time.Duration, permissions []Permission) *Service {
	return &Service{
		username:    username,
		password:    password,
		sessionTTL:  sessionTTL,
		permissions: append([]Permission(nil), permissions...),
		sessions:    make(map[string]Session),
		now:         time.Now,
	}
}

func (s *Service) Login(username, password string) (Session, error) {
	if subtle.ConstantTimeCompare([]byte(username), []byte(s.username)) != 1 {
		return Session{}, ErrInvalidCredentials
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(s.password)) != 1 {
		return Session{}, ErrInvalidCredentials
	}
	return s.IssueSession(username, s.permissions), nil
}

func (s *Service) IssueSession(username string, permissions []Permission) Session {
	session := Session{
		ID:          newSessionID(),
		Username:    username,
		Permissions: append([]Permission(nil), permissions...),
		ExpiresAt:   s.now().Add(s.sessionTTL),
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return session
}

func (s *Service) Authenticate(sessionID string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, false
	}
	if s.now().After(session.ExpiresAt) {
		delete(s.sessions, sessionID)
		return nil, false
	}
	copy := session
	return &copy, true
}

func (s *Service) Logout(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

func (s *Service) Authorize(sessionID string, permission Permission) (*Session, bool) {
	session, ok := s.Authenticate(sessionID)
	if !ok {
		return nil, false
	}
	if !HasPermission(session.Permissions, permission) {
		return nil, false
	}
	return session, true
}

func (s *Service) SetNow(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

func newSessionID() string {
	return uuid.NewString()
}
