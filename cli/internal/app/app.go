package app

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
)

type app struct {
	version string
	stdout  io.Writer
	stderr  io.Writer
}

func Run(args []string, stdout, stderr io.Writer, version string) int {
	a := &app{
		version: version,
		stdout:  stdout,
		stderr:  stderr,
	}

	root := a.rootCmd()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func (a *app) rootCmd() *cobra.Command {
	opts := config.Options{}

	root := &cobra.Command{
		Use:           "xmdm",
		Short:         "XMDM CLI",
		Version:       a.version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "path to CLI config file")
	root.PersistentFlags().StringVar(&opts.Profile, "profile", "", "named profile to use")
	root.PersistentFlags().StringVar(&opts.BaseURL, "base-url", "", "explicit server base URL")
	root.PersistentFlags().StringVar(&opts.AuthMode, "auth-mode", "", "authentication mode placeholder")
	root.PersistentFlags().StringVar(&opts.Format, "format", "", "default output format")
	root.PersistentFlags().DurationVar(&opts.Timeout, "timeout", 0, "request timeout")
	root.SetVersionTemplate("xmdm {{.Version}}\n")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		if cmd == root {
			a.printRootHelp(opts)
			return
		}
		_, _ = fmt.Fprint(a.stdout, cmd.UsageString())
	})
	root.AddCommand(a.versionCmd())
	root.AddCommand(a.configCmd(&opts))
	return root
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), a.version)
			return err
		},
	}
}

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

func (a *app) printResolved(w io.Writer, resolved config.Resolved) error {
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

func (a *app) printRootHelp(opts config.Options) {
	resolved := config.Resolved{
		ConfigPath:   strings.TrimSpace(opts.ConfigPath),
		ProfileName:  strings.TrimSpace(opts.Profile),
		BaseURL:      strings.TrimSpace(opts.BaseURL),
		AuthMode:     strings.TrimSpace(opts.AuthMode),
		OutputFormat: strings.TrimSpace(opts.Format),
	}

	fmt.Fprintf(a.stdout, "xmdm %s\n\n", a.version)
	fmt.Fprintln(a.stdout, "Usage:")
	fmt.Fprintln(a.stdout, "  xmdm [flags] <command>")
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Commands:")
	printIndentedTree(a.stdout, []string{
		"help",
		"version",
		"config",
		"  show",
		"  validate",
	})
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Global flags:")
	printIndentedTree(a.stdout, []string{
		"--config     path to CLI config file (default: ~/.config/xmdm/config.yaml)",
		"--profile    named profile to use",
		"--base-url   explicit server base URL",
		"--auth-mode  authentication mode placeholder",
		"--format     default output format",
		"--timeout    request timeout",
		"--version    print the CLI version",
	})
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Config precedence:")
	fmt.Fprintln(a.stdout, "  flags, environment variables, config file, defaults")
	fmt.Fprintln(a.stdout, "Example config:")
	fmt.Fprintln(a.stdout, "  config.yaml")
	if resolved.HasTarget() {
		fmt.Fprintln(a.stdout)
		fmt.Fprintln(a.stdout, "Current target:")
		fmt.Fprintf(a.stdout, "  %s\n", resolved.BaseURL)
	}
}

func printIndentedTree(w io.Writer, lines []string) {
	for _, line := range lines {
		fmt.Fprintln(w, "  "+line)
	}
}
