package e2e_test

import (
	"context"
	"os"
	"testing"

	"xmdm/server/internal/artifacts"
	s3store "xmdm/server/internal/artifacts/s3"
)

func newTestArtifactStore(t *testing.T) artifacts.Store {
	t.Helper()
	store, err := s3store.New(context.Background(), s3store.Config{
		Endpoint:        envOrDefault("XMDM_OBJECT_STORAGE_ENDPOINT", "http://127.0.0.1:8333"),
		Region:          envOrDefault("XMDM_OBJECT_STORAGE_REGION", "us-east-1"),
		AccessKeyID:     envOrDefault("XMDM_OBJECT_STORAGE_ACCESS_KEY", "xmdm"),
		SecretAccessKey: envOrDefault("XMDM_OBJECT_STORAGE_SECRET_KEY", "xmdm"),
		Bucket:          envOrDefault("XMDM_OBJECT_STORAGE_BUCKET", "xmdm"),
		UsePathStyle:    true,
	})
	if err != nil {
		t.Fatalf("new test artifact store: %v", err)
	}
	return store
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
