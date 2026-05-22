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
	"time"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
	"xmdm/cli/internal/session"
)

func (a *app) enrollmentCmd(opts *config.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enrollment",
		Short: "Manage enrollment tokens and bootstrap payloads",
	}
	cmd.AddCommand(a.enrollmentTokensCmd(opts))
	cmd.AddCommand(a.enrollmentQRCmd(opts))
	return cmd
}

func (a *app) enrollmentTokensCmd(opts *config.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokens",
		Short: "Manage enrollment tokens",
	}
	cmd.AddCommand(a.enrollmentIssueTokenCmd(opts))
	cmd.AddCommand(a.enrollmentValidateTokenCmd(opts))
	cmd.AddCommand(a.enrollmentConsumeTokenCmd(opts))
	cmd.AddCommand(a.enrollmentRevokeTokenCmd(opts))
	return cmd
}

func (a *app) enrollmentIssueTokenCmd(opts *config.Options) *cobra.Command {
	var ttl time.Duration
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Issue a new enrollment token",
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
			if ttl <= 0 {
				return fmt.Errorf("ttl must be greater than zero")
			}
			body, _ := json.Marshal(map[string]any{"ttlSeconds": int(ttl.Seconds())})
			payload, err := a.doEnrollmentJSON(cmd.Context(), resolved, state, http.MethodPost, "/enrollment/tokens", body)
			if err != nil {
				return err
			}
			return a.writeSuccess(cmd.OutOrStdout(), resolved, cmd.CommandPath(), map[string]any{"item": json.RawMessage(payload)})
		},
	}
	cmd.Flags().DurationVar(&ttl, "ttl", 24*time.Hour, "token time to live")
	return cmd
}

func (a *app) enrollmentValidateTokenCmd(opts *config.Options) *cobra.Command {
	return a.enrollmentLookupTokenCmd(opts, "validate", "/enrollment/tokens/validate", "validate")
}

func (a *app) enrollmentConsumeTokenCmd(opts *config.Options) *cobra.Command {
	return a.enrollmentLookupTokenCmd(opts, "consume", "/enrollment/tokens/consume", "consume")
}

func (a *app) enrollmentLookupTokenCmd(opts *config.Options, use, path, verb string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <token>",
		Short: titleWord(verb) + " an enrollment token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			if err := config.RequireTarget(resolved); err != nil {
				return err
			}
			body, _ := json.Marshal(map[string]any{"token": strings.TrimSpace(args[0])})
			payload, err := a.doEnrollmentJSON(cmd.Context(), resolved, session.State{}, http.MethodPost, path, body)
			if err != nil {
				return err
			}
			return a.writeSuccess(cmd.OutOrStdout(), resolved, cmd.CommandPath(), map[string]any{"item": json.RawMessage(payload)})
		},
	}
}

func (a *app) enrollmentRevokeTokenCmd(opts *config.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke an enrollment token",
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
			payload, err := a.doEnrollmentJSON(cmd.Context(), resolved, state, http.MethodDelete, "/enrollment/tokens/"+strings.TrimSpace(args[0]), nil)
			if err != nil {
				return err
			}
			return a.writeSuccess(cmd.OutOrStdout(), resolved, cmd.CommandPath(), map[string]any{"item": json.RawMessage(payload)})
		},
	}
}

func (a *app) enrollmentQRCmd(opts *config.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "qr",
		Short: "Generate enrollment QR payloads",
	}
	cmd.AddCommand(a.enrollmentQRPngCmd(opts))
	cmd.AddCommand(a.enrollmentQRJSONCmd(opts))
	return cmd
}

func (a *app) enrollmentQRPngCmd(opts *config.Options) *cobra.Command {
	var input qrInput
	var outputPath string
	cmd := &cobra.Command{
		Use:   "png",
		Short: "Generate the QR PNG",
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
			body, err := input.body(resolved)
			if err != nil {
				return err
			}
			payload, err := a.doEnrollmentBinary(cmd.Context(), resolved, state, http.MethodPost, "/enrollment/qr", body)
			if err != nil {
				return err
			}
			if strings.TrimSpace(outputPath) != "" {
				return os.WriteFile(strings.TrimSpace(outputPath), payload, 0o600)
			}
			_, err = cmd.OutOrStdout().Write(payload)
			return err
		},
	}
	input.bind(cmd)
	cmd.Flags().StringVar(&outputPath, "output", "", "write PNG output to a file")
	return cmd
}

func (a *app) enrollmentQRJSONCmd(opts *config.Options) *cobra.Command {
	var input qrInput
	cmd := &cobra.Command{
		Use:   "json",
		Short: "Generate the raw Android provisioning payload",
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
			body, err := input.body(resolved)
			if err != nil {
				return err
			}
			payload, err := a.doEnrollmentJSON(cmd.Context(), resolved, state, http.MethodPost, "/enrollment/qr/json", body)
			if err != nil {
				return err
			}
			return a.writeSuccess(cmd.OutOrStdout(), resolved, cmd.CommandPath(), map[string]any{"item": json.RawMessage(payload)})
		},
	}
	input.bind(cmd)
	return cmd
}

func (a *app) doEnrollmentJSON(ctx context.Context, resolved config.Resolved, state session.State, method, path string, body []byte) ([]byte, error) {
	client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := client.NewRequest(ctx, method, path, reader)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if state.IsValid() {
		req.AddCookie(&http.Cookie{Name: cookieNameOrDefault(state), Value: state.CookieValue})
	}
	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, transportFailureError(method+" "+path+" request failed", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, httpFailureError(method+" "+path+" failed", resp.StatusCode, payload)
	}
	return payload, nil
}

func (a *app) doEnrollmentBinary(ctx context.Context, resolved config.Resolved, state session.State, method, path string, body []byte) ([]byte, error) {
	return a.doEnrollmentJSON(ctx, resolved, state, method, path, body)
}

type qrInput struct {
	serverURL                 string
	enrollmentToken           string
	componentName             string
	packageURL                string
	packageChecksum           string
	deviceID                  string
	bootstrapExtras           string
	leaveAllSystemAppsEnabled bool
	skipEncryption            bool
	useMobileData             bool
}

func (q *qrInput) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&q.serverURL, "server-url", "", "server URL embedded in the payload")
	cmd.Flags().StringVar(&q.enrollmentToken, "enrollment-token", "", "enrollment token")
	cmd.Flags().StringVar(&q.componentName, "component-name", "", "device admin component name")
	cmd.Flags().StringVar(&q.packageURL, "package-url", "", "device admin package download URL")
	cmd.Flags().StringVar(&q.packageChecksum, "package-checksum", "", "device admin package checksum")
	cmd.Flags().StringVar(&q.deviceID, "device-id", "", "device id to embed")
	cmd.Flags().StringVar(&q.bootstrapExtras, "bootstrap-extras", "", "inline JSON bootstrap extras object")
	cmd.Flags().BoolVar(&q.leaveAllSystemAppsEnabled, "leave-all-system-apps-enabled", false, "leave all system apps enabled")
	cmd.Flags().BoolVar(&q.skipEncryption, "skip-encryption", false, "skip device encryption")
	cmd.Flags().BoolVar(&q.useMobileData, "use-mobile-data", false, "allow mobile data during provisioning")
	_ = cmd.MarkFlagRequired("package-url")
	_ = cmd.MarkFlagRequired("package-checksum")
}

func (q qrInput) body(resolved config.Resolved) ([]byte, error) {
	serverURL := strings.TrimSpace(q.serverURL)
	if serverURL == "" {
		serverURL = strings.TrimSpace(resolved.BaseURL)
	}
	if serverURL == "" {
		return nil, fmt.Errorf("server-url is required")
	}
	extras := map[string]any{}
	if strings.TrimSpace(q.bootstrapExtras) != "" {
		if err := json.Unmarshal([]byte(q.bootstrapExtras), &extras); err != nil {
			return nil, fmt.Errorf("invalid bootstrap extras: %w", err)
		}
	}
	payload := map[string]any{
		"serverUrl":                          serverURL,
		"enrollmentToken":                    strings.TrimSpace(q.enrollmentToken),
		"deviceAdminComponentName":           strings.TrimSpace(q.componentName),
		"deviceAdminPackageDownloadLocation": strings.TrimSpace(q.packageURL),
		"deviceAdminPackageChecksum":         strings.TrimSpace(q.packageChecksum),
		"leaveAllSystemAppsEnabled":          q.leaveAllSystemAppsEnabled,
		"skipEncryption":                     q.skipEncryption,
		"useMobileData":                      q.useMobileData,
		"deviceIdentityPolicy": map[string]any{
			"deviceId": strings.TrimSpace(q.deviceID),
		},
		"bootstrapExtras": extras,
	}
	if payload["deviceAdminComponentName"] == "" {
		delete(payload, "deviceAdminComponentName")
	}
	if payload["enrollmentToken"] == "" {
		delete(payload, "enrollmentToken")
	}
	if strings.TrimSpace(q.deviceID) == "" {
		return nil, fmt.Errorf("device-id is required")
	}
	return json.Marshal(payload)
}

func titleWord(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
