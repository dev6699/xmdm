package app

import (
	"encoding/json"
	"fmt"

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
			return a.printResolved(cmd.OutOrStdout(), resolved)
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
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "target resolved: %s\n", joined)
			return err
		},
	})
	return cmd
}

func (a *app) printResolved(w ioWriter, resolved config.Resolved) error {
	payload := map[string]any{
		"configPath":   resolved.ConfigPath,
		"profile":      resolved.ProfileName,
		"baseUrl":      resolved.BaseURL,
		"authMode":     resolved.AuthMode,
		"outputFormat": resolved.OutputFormat,
		"timeout":      resolved.Timeout.String(),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
