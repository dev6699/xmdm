package app

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
)

func (a *app) configCmd(opts *config.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect CLI configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the resolved CLI configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			return a.printResolved(resolved, cmd.CommandPath(), cmd.OutOrStdout())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Verify the server target can be resolved",
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := config.Resolve(*opts)
			if err != nil {
				return err
			}
			if err := config.RequireTarget(resolved); err != nil {
				return err
			}
			client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
			if err != nil {
				return err
			}
			joined, err := client.ResolveURL("/")
			if err != nil {
				return err
			}
			req, err := client.NewRequest(context.Background(), http.MethodGet, "/admin/login", nil)
			if err != nil {
				return err
			}
			resp, err := client.HTTP.Do(req)
			if err != nil {
				return transportFailureError("config validate request failed", err)
			}
			defer resp.Body.Close()
			payload, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				return httpFailureError("config validate failed", resp.StatusCode, payload)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "target reachable: %s (status: %d)\n", joined, resp.StatusCode)
			return err
		},
	})
	return cmd
}

func (a *app) printResolved(resolved config.Resolved, command string, w ioWriter) error {
	payload := map[string]any{
		"configPath":   resolved.ConfigPath,
		"profile":      resolved.ProfileName,
		"baseUrl":      resolved.BaseURL,
		"authMode":     resolved.AuthMode,
		"outputFormat": resolved.OutputFormat,
		"timeout":      resolved.Timeout.String(),
	}
	return a.writeSuccess(w, resolved, command, payload)
}
