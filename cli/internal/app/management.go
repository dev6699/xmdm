package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
	"xmdm/cli/internal/session"
)

var managedResourceCatalog = []resourceSpec{
	{Name: "users", Singular: "user", Path: "/users", IncludeShow: true},
	{Name: "roles", Singular: "role", Path: "/roles", IncludeShow: true},
	{Name: "groups", Singular: "group", Path: "/groups", IncludeShow: true},
	{Name: "policies", Singular: "policy", Path: "/policies", IncludeShow: true},
	{Name: "devices", Singular: "device", Path: "/devices", IncludeShow: true},
}

func (a *app) managedResourceHelpLines() []string {
	lines := make([]string, 0, len(managedResourceCatalog)*4)
	for _, spec := range managedResourceCatalog {
		lines = append(lines, spec.Name)
		lines = append(lines, "  create")
		lines = append(lines, "  update")
		lines = append(lines, "  retire")
	}
	return lines
}

func (a *app) resourceCreateCmd(opts *config.Options, spec resourceSpec) *cobra.Command {
	var input mutationInput
	cmd := &cobra.Command{
		Use:   "create",
		Short: fmt.Sprintf("Create a %s", spec.Singular),
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			if err := config.RequireTarget(resolved); err != nil {
				return err
			}
			state, err := a.loadSession()
			if err != nil {
				return err
			}
			if err := a.verifySessionAlive(state); err != nil {
				return err
			}
			body, err := input.read()
			if err != nil {
				return err
			}
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodPost, spec.Path, body)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(cmd.OutOrStdout(), item)
		},
	}
	input.bind(cmd)
	return cmd
}

func (a *app) resourceUpdateCmd(opts *config.Options, spec resourceSpec) *cobra.Command {
	var input mutationInput
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: fmt.Sprintf("Update a %s", spec.Singular),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			if err := config.RequireTarget(resolved); err != nil {
				return err
			}
			state, err := a.loadSession()
			if err != nil {
				return err
			}
			if err := a.verifySessionAlive(state); err != nil {
				return err
			}
			body, err := input.read()
			if err != nil {
				return err
			}
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodPatch, spec.Path+"/"+strings.TrimSpace(args[0]), body)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(cmd.OutOrStdout(), item)
		},
	}
	input.bind(cmd)
	return cmd
}

func (a *app) resourceRetireCmd(opts *config.Options, spec resourceSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retire <id>",
		Short: fmt.Sprintf("Retire a %s", spec.Singular),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			if err := config.RequireTarget(resolved); err != nil {
				return err
			}
			state, err := a.loadSession()
			if err != nil {
				return err
			}
			if err := a.verifySessionAlive(state); err != nil {
				return err
			}
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodDelete, spec.Path+"/"+strings.TrimSpace(args[0]), nil)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(cmd.OutOrStdout(), item)
		},
	}
	return cmd
}

func (a *app) mutateResource(ctx context.Context, resolved config.Resolved, state session.State, method, requestPath string, body []byte) (json.RawMessage, error) {
	client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := client.NewRequest(ctx, method, requestPath, reader)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: cookieNameOrDefault(state), Value: state.CookieValue})
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if len(payload) > 0 {
			return nil, fmt.Errorf("%s %s failed: %s: %s", method, requestPath, resp.Status, strings.TrimSpace(string(payload)))
		}
		return nil, fmt.Errorf("%s %s failed: %s", method, requestPath, resp.Status)
	}
	var item json.RawMessage
	if err := json.Unmarshal(payload, &item); err != nil {
		return nil, err
	}
	return item, nil
}

type mutationInput struct {
	inline string
	file   string
}

func (m *mutationInput) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&m.inline, "json", "", "inline JSON request body")
	cmd.Flags().StringVar(&m.file, "file", "", "path to a JSON request body")
}

func (m mutationInput) read() ([]byte, error) {
	if strings.TrimSpace(m.inline) != "" {
		return []byte(m.inline), nil
	}
	if strings.TrimSpace(m.file) != "" {
		return os.ReadFile(strings.TrimSpace(m.file))
	}
	return nil, fmt.Errorf("either --json or --file is required")
}
