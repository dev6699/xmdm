package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/session"
)

func TestValidateCommandTypeBodyAllowsRegisteredPluginTypes(t *testing.T) {
	transport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/admin/plugins" {
			return nil, fmt.Errorf("unexpected path %s", r.URL.Path)
		}
		return jsonResponse(r, `{"plugins":[{"enabled":true,"commandTypes":[{"type":"remote-lock"}]}]}`), nil
	})
	t.Cleanup(func() { http.DefaultTransport = transport })
	a := &app{}
	resolved := config.Resolved{BaseURL: "https://example.com", Timeout: time.Second}
	state := session.State{BaseURL: resolved.BaseURL, CookieValue: "session-cookie"}

	if err := a.validateCommandTypeBody(context.Background(), resolved, state, []byte(`{"type":"remote-lock"}`)); err != nil {
		t.Fatalf("expected plugin command type to validate, got %v", err)
	}
}

func TestValidateCommandTypeBodyRejectsUnknownPluginTypes(t *testing.T) {
	transport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/admin/plugins" {
			return nil, fmt.Errorf("unexpected path %s", r.URL.Path)
		}
		return jsonResponse(r, `{"plugins":[{"enabled":true,"commandTypes":[{"type":"remote-lock"}]}]}`), nil
	})
	t.Cleanup(func() { http.DefaultTransport = transport })
	a := &app{}
	resolved := config.Resolved{BaseURL: "https://example.com", Timeout: time.Second}
	state := session.State{BaseURL: resolved.BaseURL, CookieValue: "session-cookie"}

	err := a.validateCommandTypeBody(context.Background(), resolved, state, []byte(`{"type":"unknown-type"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported command type") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestValidateCommandTypeBodyAllowsBuiltinCompanionLaunch(t *testing.T) {
	a := &app{}
	resolved := config.Resolved{BaseURL: "https://example.com", Timeout: time.Second}
	state := session.State{BaseURL: resolved.BaseURL, CookieValue: "session-cookie"}

	if err := a.validateCommandTypeBody(context.Background(), resolved, state, []byte(`{"type":"launch_companion_app"}`)); err != nil {
		t.Fatalf("expected builtin companion launch command to validate, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
		Body:       ioNopCloser{strings.NewReader(body)},
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}
}

type ioNopCloser struct {
	*strings.Reader
}

func (c ioNopCloser) Close() error { return nil }
