package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultOutputFormat = "table"
	DefaultAuthMode     = "session"
	DefaultTimeout      = 30 * time.Second
)

type File struct {
	DefaultProfile string             `yaml:"defaultProfile"`
	DefaultFormat  string             `yaml:"defaultFormat"`
	DefaultTimeout string             `yaml:"defaultTimeout"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

type Profile struct {
	BaseURL      string `yaml:"baseUrl"`
	AuthMode     string `yaml:"authMode"`
	OutputFormat string `yaml:"outputFormat"`
	Timeout      string `yaml:"timeout"`
}

type Options struct {
	ConfigPath string
	Profile    string
	BaseURL    string
	AuthMode   string
	Format     string
	Timeout    time.Duration
}

type Resolved struct {
	ConfigPath   string
	ProfileName  string
	BaseURL      string
	AuthMode     string
	OutputFormat string
	Timeout      time.Duration
	ProfileFound bool
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "xmdm", "config.yaml"), nil
}

func EnvPath() string {
	return os.Getenv("XMDM_CONFIG")
}

func Load(path string) (File, error) {
	cfg := File{Profiles: map[string]Profile{}}
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return cfg, err
	}
	if err := parseSimpleConfig(string(data), &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Resolve(opts Options) (Resolved, error) {
	configPath := opts.ConfigPath
	explicitConfigPath := strings.TrimSpace(configPath) != ""
	if configPath == "" {
		if env := strings.TrimSpace(EnvPath()); env != "" {
			configPath = env
			explicitConfigPath = true
		} else if def, err := DefaultPath(); err == nil {
			if _, statErr := os.Stat(def); statErr == nil {
				configPath = def
			}
		}
	}

	fileCfg := File{Profiles: map[string]Profile{}}
	if configPath != "" {
		loaded, err := Load(configPath)
		if err != nil {
			if os.IsNotExist(err) && !explicitConfigPath {
				configPath = ""
			} else {
				return Resolved{}, err
			}
		} else {
			fileCfg = loaded
		}
	}

	resolved := Resolved{
		ConfigPath:   configPath,
		BaseURL:      strings.TrimSpace(opts.BaseURL),
		AuthMode:     strings.TrimSpace(opts.AuthMode),
		OutputFormat: strings.TrimSpace(opts.Format),
		Timeout:      opts.Timeout,
	}

	if resolved.OutputFormat == "" {
		resolved.OutputFormat = strings.TrimSpace(os.Getenv("XMDM_OUTPUT_FORMAT"))
	}
	if resolved.AuthMode == "" {
		resolved.AuthMode = strings.TrimSpace(os.Getenv("XMDM_AUTH_MODE"))
	}
	if resolved.BaseURL == "" {
		resolved.BaseURL = strings.TrimSpace(os.Getenv("XMDM_BASE_URL"))
	}
	if resolved.Timeout == 0 {
		if timeout := strings.TrimSpace(os.Getenv("XMDM_TIMEOUT")); timeout != "" {
			if dur, parseErr := time.ParseDuration(timeout); parseErr == nil {
				resolved.Timeout = dur
			}
		}
	}

	profileName := strings.TrimSpace(opts.Profile)
	if profileName == "" {
		profileName = strings.TrimSpace(os.Getenv("XMDM_PROFILE"))
	}
	if profileName == "" {
		profileName = strings.TrimSpace(fileCfg.DefaultProfile)
	}

	if profileName != "" {
		profile, ok := fileCfg.Profiles[profileName]
		if !ok {
			return Resolved{}, &ValidationError{Message: fmt.Sprintf("profile %q not found", profileName)}
		}
		resolved.ProfileName = profileName
		resolved.ProfileFound = true
		if resolved.BaseURL == "" {
			resolved.BaseURL = strings.TrimSpace(profile.BaseURL)
		}
		if resolved.AuthMode == "" {
			resolved.AuthMode = strings.TrimSpace(profile.AuthMode)
		}
		if resolved.OutputFormat == "" {
			resolved.OutputFormat = strings.TrimSpace(profile.OutputFormat)
		}
		if resolved.Timeout == 0 {
			if profile.Timeout != "" {
				if dur, parseErr := time.ParseDuration(profile.Timeout); parseErr == nil {
					resolved.Timeout = dur
				}
			}
		}
	}

	if resolved.OutputFormat == "" {
		if strings.TrimSpace(fileCfg.DefaultFormat) != "" {
			resolved.OutputFormat = strings.TrimSpace(fileCfg.DefaultFormat)
		} else {
			resolved.OutputFormat = DefaultOutputFormat
		}
	}
	if resolved.AuthMode == "" {
		resolved.AuthMode = DefaultAuthMode
	}
	if resolved.Timeout == 0 {
		if strings.TrimSpace(fileCfg.DefaultTimeout) != "" {
			if dur, parseErr := time.ParseDuration(strings.TrimSpace(fileCfg.DefaultTimeout)); parseErr == nil {
				resolved.Timeout = dur
			}
		}
	}
	if resolved.Timeout == 0 {
		resolved.Timeout = DefaultTimeout
	}

	return resolved, nil
}

func RequireTarget(resolved Resolved) error {
	if strings.TrimSpace(resolved.BaseURL) == "" {
		return &ValidationError{Message: "a profile or --base-url is required before running a networked command"}
	}
	return nil
}

func (r Resolved) HasTarget() bool {
	return strings.TrimSpace(r.BaseURL) != ""
}

func parseSimpleConfig(src string, cfg *File) error {
	var section string
	var profileName string
	lines := strings.Split(src, "\n")
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "  ") && strings.HasPrefix(trimmed, "profiles:"):
			return fmt.Errorf("invalid indentation before profiles section")
		case trimmed == "profiles:":
			section = "profiles"
			profileName = ""
			continue
		}

		if section == "" {
			key, value, ok := cutKV(trimmed)
			if !ok {
				return fmt.Errorf("invalid top-level config line: %q", line)
			}
			switch key {
			case "defaultProfile":
				cfg.DefaultProfile = value
			case "defaultFormat":
				cfg.DefaultFormat = value
			case "defaultTimeout":
				cfg.DefaultTimeout = value
			case "profiles":
				section = "profiles"
			default:
				return fmt.Errorf("unknown top-level config key %q", key)
			}
			continue
		}

		if section == "profiles" {
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
				nameLine := strings.TrimSpace(line)
				if !strings.HasSuffix(nameLine, ":") {
					return fmt.Errorf("invalid profile name line: %q", line)
				}
				profileName = strings.TrimSuffix(nameLine, ":")
				if profileName == "" {
					return fmt.Errorf("empty profile name")
				}
				if cfg.Profiles == nil {
					cfg.Profiles = map[string]Profile{}
				}
				cfg.Profiles[profileName] = Profile{}
				continue
			}
			if profileName == "" {
				return fmt.Errorf("profile fields must follow a profile name")
			}
			if !strings.HasPrefix(line, "    ") {
				return fmt.Errorf("invalid profile field indentation: %q", line)
			}
			key, value, ok := cutKV(strings.TrimSpace(line))
			if !ok {
				return fmt.Errorf("invalid profile field line: %q", line)
			}
			p := cfg.Profiles[profileName]
			switch key {
			case "baseUrl":
				p.BaseURL = value
			case "authMode":
				p.AuthMode = value
			case "outputFormat":
				p.OutputFormat = value
			case "timeout":
				p.Timeout = value
			default:
				return fmt.Errorf("unknown profile field %q", key)
			}
			cfg.Profiles[profileName] = p
			continue
		}
	}

	return nil
}

func cutKV(line string) (key, value string, ok bool) {
	keyPart, valuePart, found := strings.Cut(line, ":")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(keyPart)
	value = strings.TrimSpace(valuePart)
	value = strings.Trim(value, `"'`)
	return key, value, true
}
