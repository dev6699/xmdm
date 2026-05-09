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

func (a *app) deviceInspectCmd(opts *config.Options) *cobra.Command {
	var deviceSecret string
	var limit int
	cmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Inspect device health and activity",
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

			deviceID := strings.TrimSpace(args[0])
			if deviceID == "" {
				return fmt.Errorf("device id is required")
			}
			if limit <= 0 {
				limit = 5
			}

			deviceItem, err := a.fetchResourceItemByID(cmd.Context(), resolved, state, resourceSpec{Name: "devices", Singular: "device", Path: "/devices", IncludeShow: true}, deviceID)
			if err != nil {
				return err
			}
			deviceName := rawDeviceField(deviceItem, "name")
			logs, err := a.fetchResourceItems(cmd.Context(), resolved, resourceSpec{Name: "logs", Path: "/logs", ListField: "logs"}, state)
			if err != nil {
				return err
			}
			info, err := a.fetchResourceItems(cmd.Context(), resolved, resourceSpec{Name: "device-info", Path: "/device-info", ListField: "deviceInfo"}, state)
			if err != nil {
				return err
			}
			commands, err := a.fetchResourceItems(cmd.Context(), resolved, resourceSpec{Name: "commands", Path: "/admin/commands", ListField: "commands"}, state)
			if err != nil {
				return err
			}
			auditEvents, err := a.fetchResourceItems(cmd.Context(), resolved, resourceSpec{Name: "audit", Path: "/admin/audit", ListField: "events"}, state)
			if err != nil {
				return err
			}

			view := map[string]any{
				"device":     deviceItem,
				"logs":       trimResourceItems(logs, func(raw json.RawMessage) bool { return rawDeviceField(raw, "deviceId") == deviceName }, limit),
				"deviceInfo": trimResourceItems(info, func(raw json.RawMessage) bool { return rawDeviceField(raw, "deviceId") == deviceName }, limit),
				"commands":   trimResourceItems(commands, func(raw json.RawMessage) bool { return rawDeviceField(raw, "deviceId") == deviceID }, limit),
				"audit":      trimResourceItems(auditEvents, func(raw json.RawMessage) bool { return rawAuditMatchesDevice(raw, deviceID) }, limit),
			}

			if strings.TrimSpace(deviceSecret) != "" {
				snapshot, err := a.fetchDeviceSnapshot(cmd.Context(), resolved, deviceName, deviceSecret)
				if err != nil {
					view["configError"] = err.Error()
				} else {
					view["config"] = snapshot
				}
			}

			return a.writeSuccess(cmd.OutOrStdout(), resolved, cmd.CommandPath(), view)
		},
	}
	cmd.Flags().StringVar(&deviceSecret, "device-secret", "", "device secret for signed config inspection")
	cmd.Flags().IntVar(&limit, "limit", 5, "maximum records to show per section")
	return cmd
}

func (a *app) fetchResourceItemByID(ctx context.Context, resolved config.Resolved, state session.State, spec resourceSpec, id string) (json.RawMessage, error) {
	items, err := a.fetchResourceItems(ctx, resolved, spec, state)
	if err != nil {
		return nil, err
	}
	item, ok := findResourceItem(items, id)
	if !ok {
		return nil, fmt.Errorf("%s %q not found", spec.Singular, id)
	}
	return item, nil
}

func (a *app) fetchDeviceSnapshot(ctx context.Context, resolved config.Resolved, deviceID, deviceSecret string) (json.RawMessage, error) {
	client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, http.MethodGet, "/devices/"+strings.TrimSpace(deviceID)+"/config", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-XMDM-Device-Secret", strings.TrimSpace(deviceSecret))
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, transportFailureError("device snapshot request failed", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, httpFailureError("device config fetch failed", resp.StatusCode, payload)
	}
	var snapshot json.RawMessage
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func trimResourceItems(items []json.RawMessage, keep func(json.RawMessage) bool, limit int) []json.RawMessage {
	if limit <= 0 {
		return nil
	}
	out := make([]json.RawMessage, 0, limit)
	for _, item := range items {
		if keep != nil && !keep(item) {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func rawDeviceField(raw json.RawMessage, field string) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	value, _ := payload[field]
	return strings.TrimSpace(fmt.Sprint(value))
}

func rawAuditMatchesDevice(raw json.RawMessage, deviceID string) bool {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	if strings.TrimSpace(fmt.Sprint(payload["resourceType"])) != "devices" {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(payload["resourceId"])) == strings.TrimSpace(deviceID)
}
