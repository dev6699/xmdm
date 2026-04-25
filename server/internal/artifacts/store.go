package artifacts

import (
	"context"
	"io"
)

// Store persists and deletes binary artifact objects.
type Store interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string, contentLength int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
