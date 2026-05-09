package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
	"xmdm/cli/internal/session"
)

func (a *app) authCmd(opts *config.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate and manage admin sessions",
	}

	cmd.AddCommand(a.loginCmd(opts))
	cmd.AddCommand(a.whoamiCmd())
	cmd.AddCommand(a.logoutCmd())
	return cmd
}

func (a *app) loginCmd(opts *config.Options) *cobra.Command {
	var username string
	var password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in and save the admin session cookie",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
				return fmt.Errorf("username and password are required")
			}

			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			if err := config.RequireTarget(resolved); err != nil {
				return err
			}

			created, err := a.performLogin(resolved, username, password)
			if err != nil {
				return err
			}
			path, err := session.DefaultPath()
			if err != nil {
				return err
			}
			if err := session.Save(path, created); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "logged in as %s\n", created.Username)
			return err
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "admin username")
	cmd.Flags().StringVar(&password, "password", "", "admin password")
	return cmd
}

func (a *app) whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the active admin session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := a.loadSession()
			if err != nil {
				return err
			}
			if err := a.verifySessionAlive(state); err != nil {
				return err
			}

			me, err := a.fetchCurrentUser(state)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "session user: %s\nbase URL: %s\n", me.User, state.BaseURL)
			return err
		},
	}
}

func (a *app) logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear the active admin session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := a.loadSession()
			if err != nil {
				return err
			}
			if err := a.performLogout(state); err != nil {
				return err
			}
			path, err := session.DefaultPath()
			if err != nil {
				return err
			}
			if err := session.Clear(path); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "logged out")
			return err
		},
	}
}

func (a *app) performLogin(resolved config.Resolved, username, password string) (session.State, error) {
	client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
	if err != nil {
		return session.State{}, err
	}

	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)

	req, err := client.NewRequest(context.Background(), http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	if err != nil {
		return session.State{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := *client.HTTP
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return session.State{}, transportFailureError("login request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		payload, _ := io.ReadAll(resp.Body)
		return session.State{}, httpFailureError("login failed", resp.StatusCode, payload)
	}

	cookie := findCookie(resp.Cookies(), session.CookieName)
	if cookie == nil {
		cookie = findCookieFromHeader(resp.Header.Values("Set-Cookie"), session.CookieName)
	}
	if cookie == nil {
		return session.State{}, fmt.Errorf("login succeeded but session cookie was not returned")
	}

	return session.State{
		BaseURL:     resolved.BaseURL,
		ProfileName: resolved.ProfileName,
		Username:    username,
		CookieName:  cookie.Name,
		CookieValue: cookie.Value,
		ExpiresAt:   cookie.Expires.UTC(),
		SavedAt:     time.Now().UTC(),
	}, nil
}

func (a *app) fetchCurrentUser(state session.State) (*meResponse, error) {
	client, err := httpclient.New(state.BaseURL, 30*time.Second)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/admin/me", nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{Name: cookieNameOrDefault(state), Value: state.CookieValue})

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, transportFailureError("whoami request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return nil, httpFailureError("whoami failed", resp.StatusCode, payload)
	}

	var payload meResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (a *app) performLogout(state session.State) error {
	client, err := httpclient.New(state.BaseURL, 30*time.Second)
	if err != nil {
		return err
	}
	req, err := client.NewRequest(context.Background(), http.MethodPost, "/admin/logout", nil)
	if err != nil {
		return err
	}
	req.AddCookie(&http.Cookie{Name: cookieNameOrDefault(state), Value: state.CookieValue})

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return transportFailureError("logout request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(resp.Body)
		return httpFailureError("logout failed", resp.StatusCode, payload)
	}
	return nil
}

func (a *app) verifySessionAlive(state session.State) error {
	if state.ExpiresAt.IsZero() {
		return nil
	}
	if time.Now().UTC().After(state.ExpiresAt) {
		return fmt.Errorf("stored session expired; run auth login again")
	}
	return nil
}

func (a *app) loadSession() (session.State, error) {
	path, err := session.DefaultPath()
	if err != nil {
		return session.State{}, err
	}
	state, err := session.Load(path)
	if err != nil {
		return session.State{}, err
	}
	if !state.IsValid() {
		return session.State{}, fmt.Errorf("no active session found; run auth login first")
	}
	if state.CookieName == "" {
		state.CookieName = session.CookieName
	}
	return state, nil
}

func cookieNameOrDefault(s session.State) string {
	if strings.TrimSpace(s.CookieName) == "" {
		return session.CookieName
	}
	return s.CookieName
}

type meResponse struct {
	User string `json:"user"`
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func findCookieFromHeader(headers []string, name string) *http.Cookie {
	for _, raw := range headers {
		if cookie := parseCookieHeader(raw); cookie != nil && cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func parseCookieHeader(raw string) *http.Cookie {
	parts := strings.Split(raw, ";")
	if len(parts) == 0 {
		return nil
	}
	key, value, ok := strings.Cut(strings.TrimSpace(parts[0]), "=")
	if !ok || key == "" {
		return nil
	}
	return &http.Cookie{Name: key, Value: value}
}
