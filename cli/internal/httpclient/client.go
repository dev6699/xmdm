package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Client struct {
	BaseURL *url.URL
	HTTP    *http.Client
}

func New(baseURL string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base url must include scheme and host")
	}
	if parsed.Path != "" && !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}

	return &Client{
		BaseURL: parsed,
		HTTP: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) NewRequest(ctx context.Context, method, requestPath string, body io.Reader) (*http.Request, error) {
	joined, err := c.ResolveURL(requestPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, joined, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "xmdm-cli")
	return req, nil
}

func (c *Client) ResolveURL(requestPath string) (string, error) {
	if c == nil || c.BaseURL == nil {
		return "", fmt.Errorf("client base url is not configured")
	}
	rel := &url.URL{Path: normalizePath(requestPath)}
	return c.BaseURL.ResolveReference(rel).String(), nil
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean("/"+p), "/")
}
