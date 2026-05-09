package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CookieName = "xmdm_session"

type State struct {
	BaseURL     string    `json:"baseUrl"`
	ProfileName string    `json:"profile,omitempty"`
	Username    string    `json:"username"`
	CookieName  string    `json:"cookieName"`
	CookieValue string    `json:"cookieValue"`
	ExpiresAt   time.Time `json:"expiresAt"`
	SavedAt     time.Time `json:"savedAt"`
}

func DefaultPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("XMDM_SESSION_FILE")); override != "" {
		return override, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xmdm", "session.json"), nil
}

func Load(path string) (State, error) {
	var state State
	if strings.TrimSpace(path) == "" {
		return state, errors.New("session path is required")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func Save(path string, state State) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("session path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if state.CookieName == "" {
		state.CookieName = CookieName
	}
	if state.SavedAt.IsZero() {
		state.SavedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Clean(path), data, 0o600)
}

func Clear(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("session path is required")
	}
	if err := os.Remove(filepath.Clean(path)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s State) IsValid() bool {
	return strings.TrimSpace(s.BaseURL) != "" && strings.TrimSpace(s.CookieValue) != ""
}
