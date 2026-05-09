package app

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
)

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
	root.AddCommand(a.authCmd(&opts))
	root.AddCommand(a.configCmd(&opts))
	root.AddCommand(a.resourceCmds(&opts)...)
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
	printIndentedTree(a.stdout, append([]string{
		"help",
		"version",
		"auth",
		"  login",
		"  whoami",
		"  logout",
		"config",
		"  show",
		"  validate",
	}, append(a.resourceHelpLines(), a.managedResourceHelpLines()...)...))
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

func printIndentedTree(w ioWriter, lines []string) {
	for _, line := range lines {
		fmt.Fprintln(w, "  "+line)
	}
}

type ioWriter interface {
	Write([]byte) (int, error)
}
