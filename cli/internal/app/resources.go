package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
	"xmdm/cli/internal/session"
)

type resourceSpec struct {
	Name        string
	Singular    string
	Path        string
	ListField   string
	IncludeShow bool
}

var resourceCatalog = []resourceSpec{
	{Name: "users", Singular: "user", Path: "/users", IncludeShow: true},
	{Name: "roles", Singular: "role", Path: "/roles", IncludeShow: true},
	{Name: "groups", Singular: "group", Path: "/groups", IncludeShow: true},
	{Name: "policies", Singular: "policy", Path: "/policies", IncludeShow: true},
	{Name: "apps", Singular: "app", Path: "/apps", IncludeShow: true},
	{Name: "files", Singular: "file", Path: "/files", IncludeShow: true},
	{Name: "managed-files", Singular: "managed file", Path: "/managed-files", IncludeShow: true},
	{Name: "certificates", Singular: "certificate", Path: "/certificates", IncludeShow: true},
	{Name: "devices", Singular: "device", Path: "/devices", IncludeShow: true},
	{Name: "commands", Singular: "command", Path: "/admin/commands", ListField: "commands", IncludeShow: true},
	{Name: "logs", Singular: "log", Path: "/logs", ListField: "logs", IncludeShow: true},
	{Name: "device-info", Singular: "device info entry", Path: "/device-info", ListField: "deviceInfo", IncludeShow: true},
	{Name: "audit", Singular: "audit event", Path: "/admin/audit", ListField: "events", IncludeShow: true},
}

func (a *app) resourceCmds(opts *config.Options) []*cobra.Command {
	cmds := make([]*cobra.Command, 0, len(resourceCatalog))
	for _, spec := range resourceCatalog {
		cmds = append(cmds, a.resourceCmd(opts, spec))
	}
	return cmds
}

func (a *app) resourceHelpLines() []string {
	lines := make([]string, 0, len(resourceCatalog)*3)
	for _, spec := range resourceCatalog {
		lines = append(lines, spec.Name)
		lines = append(lines, "  list")
		if spec.IncludeShow {
			lines = append(lines, "  show")
		}
	}
	return lines
}

func (a *app) resourceCmd(opts *config.Options, spec resourceSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   spec.Name,
		Short: fmt.Sprintf("Inspect %s", spec.Name),
	}
	cmd.AddCommand(a.resourceListCmd(opts, spec))
	if spec.IncludeShow {
		cmd.AddCommand(a.resourceShowCmd(opts, spec))
	}
	if isManagedResource(spec.Name) {
		cmd.AddCommand(a.resourceCreateCmd(opts, spec))
		cmd.AddCommand(a.resourceUpdateCmd(opts, spec))
		cmd.AddCommand(a.resourceRetireCmd(opts, spec))
	}
	if cmds := a.contentCmds(opts, spec); len(cmds) > 0 {
		cmd.AddCommand(cmds...)
	}
	return cmd
}

func (a *app) resourceListCmd(opts *config.Options, spec resourceSpec) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %s", spec.Name),
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
			items, err := a.fetchResourceItems(cmd.Context(), resolved, spec, state)
			if err != nil {
				return err
			}
			return a.printListEnvelope(cmd.OutOrStdout(), items)
		},
	}
}

func (a *app) resourceShowCmd(opts *config.Options, spec resourceSpec) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: fmt.Sprintf("Show a %s", spec.Singular),
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
			items, err := a.fetchResourceItems(cmd.Context(), resolved, spec, state)
			if err != nil {
				return err
			}
			item, ok := findResourceItem(items, args[0])
			if !ok {
				return fmt.Errorf("%s %q not found", spec.Singular, args[0])
			}
			return a.printShowEnvelope(cmd.OutOrStdout(), item)
		},
	}
}

func (a *app) fetchResourceItems(ctx context.Context, resolved config.Resolved, spec resourceSpec, state session.State) ([]json.RawMessage, error) {
	client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, spec.Path, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{Name: cookieNameOrDefault(state), Value: state.CookieValue})
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s list failed: %s", spec.Name, resp.Status)
	}
	return decodeResourceItems(resp.Body, spec.ListField)
}

func decodeResourceItems(body io.Reader, field string) ([]json.RawMessage, error) {
	if field == "" {
		var items []json.RawMessage
		if err := json.NewDecoder(body).Decode(&items); err != nil {
			return nil, err
		}
		return items, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.NewDecoder(body).Decode(&envelope); err != nil {
		return nil, err
	}
	rawItems, ok := envelope[field]
	if !ok {
		return nil, fmt.Errorf("response missing %q field", field)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func findResourceItem(items []json.RawMessage, id string) (json.RawMessage, bool) {
	for _, raw := range items {
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.ID) == strings.TrimSpace(id) {
			return raw, true
		}
	}
	return nil, false
}

func (a *app) printListEnvelope(w ioWriter, items []json.RawMessage) error {
	payload := map[string]any{
		"items":  items,
		"count":  len(items),
		"cursor": nil,
	}
	return a.writeIndentedJSON(w, payload)
}

func (a *app) printShowEnvelope(w ioWriter, item json.RawMessage) error {
	return a.writeIndentedJSON(w, map[string]any{"item": item})
}

func (a *app) writeIndentedJSON(w ioWriter, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func isManagedResource(name string) bool {
	switch name {
	case "users", "roles", "groups", "policies", "devices":
		return true
	default:
		return false
	}
}
