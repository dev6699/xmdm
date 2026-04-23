package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const SessionCookieName = "xmdm_session"

var ErrInvalidCredentials = errors.New("invalid credentials")

type Session struct {
	ID        string
	Username  string
	ExpiresAt time.Time
}

type Service struct {
	username  string
	password  string
	sessionTTL time.Duration

	mu       sync.Mutex
	sessions map[string]Session
	now      func() time.Time
}

func NewService(username, password string, sessionTTL time.Duration) *Service {
	return &Service{
		username:   username,
		password:   password,
		sessionTTL: sessionTTL,
		sessions:   make(map[string]Session),
		now:        time.Now,
	}
}

func (s *Service) Login(username, password string) (Session, error) {
	if subtle.ConstantTimeCompare([]byte(username), []byte(s.username)) != 1 {
		return Session{}, ErrInvalidCredentials
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(s.password)) != 1 {
		return Session{}, ErrInvalidCredentials
	}
	session := Session{
		ID:        newSessionID(),
		Username:  username,
		ExpiresAt: s.now().Add(s.sessionTTL),
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return session, nil
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

func (s *Service) SetNow(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

func newSessionID() string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}
