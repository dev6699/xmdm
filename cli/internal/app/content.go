package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"xmdm/cli/internal/config"
	"xmdm/cli/internal/httpclient"
	"xmdm/cli/internal/session"
)

func (a *app) contentCmds(opts *config.Options, spec resourceSpec) []*cobra.Command {
	switch spec.Name {
	case "files":
		return []*cobra.Command{
			a.uploadContentCmd(opts, "files", spec.Singular, spec.Path),
			a.retireContentCmd(opts, "files", spec.Singular, spec.Path),
		}
	case "certificates":
		return []*cobra.Command{
			a.uploadContentCmd(opts, "certificates", spec.Singular, spec.Path),
			a.retireContentCmd(opts, "certificates", spec.Singular, spec.Path),
		}
	case "managed-files":
		return []*cobra.Command{
			a.managedFileCreateCmd(opts),
			a.managedFileRetireCmd(opts),
		}
	case "apps":
		return []*cobra.Command{a.appVersionsCmd(opts)}
	default:
		return nil
	}
}

func (a *app) contentHelpLines() []string {
	return []string{
		"files",
		"  create",
		"  retire",
		"managed-files",
		"  create",
		"  retire",
		"certificates",
		"  create",
		"  retire",
		"apps",
		"  versions",
		"    publish",
	}
}

func (a *app) uploadContentCmd(opts *config.Options, resourceName, singular, path string) *cobra.Command {
	var input uploadInput
	cmd := &cobra.Command{
		Use:   "create",
		Short: fmt.Sprintf("Upload a %s", singular),
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
			item, err := a.uploadArtifact(cmd.Context(), resolved, state, path, input)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), item)
		},
	}
	input.bind(cmd)
	cmd.Short = fmt.Sprintf("Upload a %s", singular)
	cmd.Long = fmt.Sprintf("Upload a %s and create the matching %s record.", singular, resourceName)
	return cmd
}

func (a *app) retireContentCmd(opts *config.Options, resourceName, singular, path string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retire <id>",
		Short: fmt.Sprintf("Retire a %s", singular),
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
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodDelete, path+"/"+strings.TrimSpace(args[0]), nil)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), item)
		},
	}
	cmd.Long = fmt.Sprintf("Retire a %s record.", resourceName)
	return cmd
}

func (a *app) managedFileCreateCmd(opts *config.Options) *cobra.Command {
	var input managedFileInput
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a managed file",
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
			body, err := input.body()
			if err != nil {
				return err
			}
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodPost, "/managed-files", body)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), item)
		},
	}
	input.bind(cmd)
	return cmd
}

func (a *app) managedFileRetireCmd(opts *config.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retire <id>",
		Short: "Retire a managed file",
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
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodDelete, "/managed-files/"+strings.TrimSpace(args[0]), nil)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), item)
		},
	}
	return cmd
}

func (a *app) appVersionsCmd(opts *config.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Manage app versions",
	}
	cmd.AddCommand(a.publishAppVersionCmd(opts))
	return cmd
}

func (a *app) publishAppVersionCmd(opts *config.Options) *cobra.Command {
	var input appVersionInput
	cmd := &cobra.Command{
		Use:   "publish <app-id>",
		Short: "Publish an app version",
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
			body, err := input.body(true)
			if err != nil {
				return err
			}
			item, err := a.mutateResource(cmd.Context(), resolved, state, http.MethodPost, "/apps/"+strings.TrimSpace(args[0])+"/versions", body)
			if err != nil {
				return err
			}
			return a.printShowEnvelope(resolved, cmd.CommandPath(), cmd.OutOrStdout(), item)
		},
	}
	input.bind(cmd)
	return cmd
}

func (a *app) uploadArtifact(ctx context.Context, resolved config.Resolved, state session.State, path string, input uploadInput) (json.RawMessage, error) {
	source, err := os.ReadFile(strings.TrimSpace(input.source))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.name) == "" {
		input.name = filepath.Base(strings.TrimSpace(input.source))
	}
	if strings.TrimSpace(input.storageKey) == "" {
		return nil, fmt.Errorf("--storage-key is required")
	}
	if strings.TrimSpace(input.mimeType) == "" {
		return nil, fmt.Errorf("--mime-type is required")
	}
	computedChecksum := sha256Base64URL(source)
	if strings.TrimSpace(input.checksum) == "" {
		input.checksum = computedChecksum
	} else if strings.TrimSpace(input.checksum) != computedChecksum {
		return nil, fmt.Errorf("--checksum does not match the source file")
	}
	if input.sizeBytes == 0 {
		input.sizeBytes = int64(len(source))
	}
	if input.sizeBytes != int64(len(source)) {
		return nil, fmt.Errorf("--size-bytes must match the source file size")
	}

	client, err := httpclient.New(resolved.BaseURL, resolved.Timeout)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := map[string]string{
		"name":       input.name,
		"storageKey": input.storageKey,
		"checksum":   input.checksum,
		"mimeType":   input.mimeType,
		"sizeBytes":  strconv.FormatInt(input.sizeBytes, 10),
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := w.CreateFormFile("file", filepath.Base(strings.TrimSpace(input.source)))
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(source); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := client.NewRequest(ctx, http.MethodPost, path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: cookieNameOrDefault(state), Value: state.CookieValue})

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, transportFailureError("content upload request failed", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if len(payload) > 0 {
			return nil, fmt.Errorf("%s failed: %s: %s", path, resp.Status, strings.TrimSpace(string(payload)))
		}
		return nil, fmt.Errorf("%s failed: %s", path, resp.Status)
	}
	var item json.RawMessage
	if err := json.Unmarshal(payload, &item); err != nil {
		return nil, err
	}
	return item, nil
}

type uploadInput struct {
	name       string
	storageKey string
	source     string
	mimeType   string
	checksum   string
	sizeBytes  int64
}

func (u *uploadInput) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&u.name, "name", "", "record name")
	cmd.Flags().StringVar(&u.storageKey, "storage-key", "", "artifact storage key")
	cmd.Flags().StringVar(&u.source, "source", "", "path to the source file")
	cmd.Flags().StringVar(&u.mimeType, "mime-type", "", "artifact mime type")
	cmd.Flags().StringVar(&u.checksum, "checksum", "", "artifact checksum")
	cmd.Flags().Int64Var(&u.sizeBytes, "size-bytes", 0, "artifact size in bytes")
	_ = cmd.MarkFlagRequired("storage-key")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("mime-type")
}

type managedFileInput struct {
	fileID           string
	path             string
	replaceVariables bool
}

func (m *managedFileInput) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&m.fileID, "file-id", "", "backing file id")
	cmd.Flags().StringVar(&m.path, "path", "", "managed file path")
	cmd.Flags().BoolVar(&m.replaceVariables, "replace-variables", true, "expand template variables")
	_ = cmd.MarkFlagRequired("file-id")
	_ = cmd.MarkFlagRequired("path")
}

func (m managedFileInput) body() ([]byte, error) {
	if strings.TrimSpace(m.fileID) == "" || strings.TrimSpace(m.path) == "" {
		return nil, fmt.Errorf("file id and path are required")
	}
	payload := map[string]any{
		"fileId":           strings.TrimSpace(m.fileID),
		"path":             strings.TrimSpace(m.path),
		"replaceVariables": m.replaceVariables,
	}
	return json.Marshal(payload)
}

type appVersionInput struct {
	versionName string
	versionCode int64
	artifactID  string
	checksum    string
}

func (a *appVersionInput) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&a.versionName, "version-name", "", "version name")
	cmd.Flags().Int64Var(&a.versionCode, "version-code", 0, "version code")
	cmd.Flags().StringVar(&a.artifactID, "artifact-id", "", "artifact id")
	cmd.Flags().StringVar(&a.checksum, "checksum", "", "artifact checksum")
	_ = cmd.MarkFlagRequired("version-name")
	_ = cmd.MarkFlagRequired("version-code")
	_ = cmd.MarkFlagRequired("artifact-id")
	_ = cmd.MarkFlagRequired("checksum")
}

func (a appVersionInput) body(publish bool) ([]byte, error) {
	payload := map[string]any{
		"versionName": strings.TrimSpace(a.versionName),
		"versionCode": a.versionCode,
		"artifactId":  strings.TrimSpace(a.artifactID),
		"checksum":    strings.TrimSpace(a.checksum),
		"publish":     publish,
	}
	if payload["versionName"] == "" || payload["versionCode"] == int64(0) || payload["artifactId"] == "" || payload["checksum"] == "" {
		return nil, fmt.Errorf("version name, version code, artifact id, and checksum are required")
	}
	return json.Marshal(payload)
}

func sha256Base64URL(content []byte) string {
	sum := sha256.Sum256(content)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
