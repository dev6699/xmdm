package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
)

func (a *app) commandListCmd(opts *config.Options) *cobra.Command {
	var deviceID string
	var commandType string
	var status string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent commands",
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
			items, err := a.fetchResourceItems(cmd.Context(), resolved, resourceSpec{Name: "commands", Path: "/admin/commands", ListField: "commands"}, state)
			if err != nil {
				return err
			}
			items = filterCommandItems(items, deviceID, commandType, status, limit)
			return a.printListEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), items)
		},
	}
	cmd.Flags().StringVar(&deviceID, "device-id", "", "filter by device id")
	cmd.Flags().StringVar(&commandType, "type", "", "filter by command type")
	cmd.Flags().StringVar(&status, "status", "", "filter by command status")
	cmd.Flags().IntVar(&limit, "limit", 25, "maximum commands to show")
	return cmd
}

func (a *app) commandSendCmd(opts *config.Options) *cobra.Command {
	var input mutationInput
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Create and send commands",
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
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodPost, "/admin/commands", body)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), item)
		},
	}
	input.bind(cmd)
	cmd.Short = "Create and send commands"
	cmd.Long = "Create a command request through the admin command queue."
	return cmd
}

func (a *app) commandShowCmd(opts *config.Options) *cobra.Command {
	var deviceID string
	var commandType string
	var status string
	var limit int
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a command",
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
			items, err := a.fetchResourceItems(cmd.Context(), resolved, resourceSpec{Name: "commands", Path: "/admin/commands", ListField: "commands"}, state)
			if err != nil {
				return err
			}
			items = filterCommandItems(items, deviceID, commandType, status, limit)
			item, ok := findResourceItem(items, args[0])
			if !ok {
				return fmt.Errorf("command %q not found", args[0])
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), item)
		},
	}
	cmd.Flags().StringVar(&deviceID, "device-id", "", "filter by device id before selecting")
	cmd.Flags().StringVar(&commandType, "type", "", "filter by command type before selecting")
	cmd.Flags().StringVar(&status, "status", "", "filter by command status before selecting")
	cmd.Flags().IntVar(&limit, "limit", 25, "maximum commands to inspect")
	return cmd
}

func (a *app) commandAckCmd(opts *config.Options) *cobra.Command {
	var deviceSecret string
	var status string
	var message string
	var details string
	cmd := &cobra.Command{
		Use:   "ack <device-id> <command-id>",
		Short: "Record a device acknowledgement",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			if err := config.RequireTarget(resolved); err != nil {
				return err
			}
			deviceID := strings.TrimSpace(args[0])
			commandID := strings.TrimSpace(args[1])
			if deviceID == "" || commandID == "" {
				return fmt.Errorf("device id and command id are required")
			}
			if strings.TrimSpace(deviceSecret) == "" {
				return fmt.Errorf("--device-secret is required")
			}
			if strings.TrimSpace(status) == "" {
				status = "acked"
			}
			body := map[string]any{
				"status": status,
			}
			if strings.TrimSpace(message) != "" {
				body["message"] = message
			}
			if strings.TrimSpace(details) != "" {
				var parsed map[string]any
				if err := json.Unmarshal([]byte(details), &parsed); err != nil {
					return fmt.Errorf("invalid details: %w", err)
				}
				body["details"] = parsed
			}
			payload, err := a.doDeviceJSON(cmd.Context(), resolved, deviceID, deviceSecret, http.MethodPost, "/devices/"+deviceID+"/commands/"+commandID+"/ack", body)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), payload)
		},
	}
	cmd.Flags().StringVar(&deviceSecret, "device-secret", "", "device secret used for acknowledgement")
	cmd.Flags().StringVar(&status, "status", "acked", "acknowledgement status")
	cmd.Flags().StringVar(&message, "message", "", "acknowledgement message")
	cmd.Flags().StringVar(&details, "details", "", "acknowledgement details as JSON")
	return cmd
}

func filterCommandItems(items []json.RawMessage, deviceID, commandType, status string, limit int) []json.RawMessage {
	filtered := make([]json.RawMessage, 0, len(items))
	for _, raw := range items {
		if strings.TrimSpace(deviceID) != "" && rawDeviceField(raw, "deviceId") != strings.TrimSpace(deviceID) {
			continue
		}
		if strings.TrimSpace(commandType) != "" && rawDeviceField(raw, "type") != strings.TrimSpace(commandType) {
			continue
		}
		if strings.TrimSpace(status) != "" && rawDeviceField(raw, "status") != strings.TrimSpace(status) {
			continue
		}
		filtered = append(filtered, raw)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func (a *app) doDeviceJSON(ctx context.Context, resolved config.Resolved, deviceID, deviceSecret, method, path string, body map[string]any) (json.RawMessage, error) {
	client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
	if err != nil {
		return nil, err
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := client.NewRequest(ctx, method, path, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-XMDM-Device-Secret", strings.TrimSpace(deviceSecret))
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, transportFailureError(path+" request failed", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, httpFailureError(path+" failed", resp.StatusCode, payload)
	}
	var item json.RawMessage
	if err := json.Unmarshal(payload, &item); err != nil {
		return nil, err
	}
	return item, nil
}
