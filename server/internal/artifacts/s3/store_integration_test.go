package s3store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSeaweedFSIntegration(t *testing.T) {
	if os.Getenv("XMDM_RUN_OBJECT_STORAGE_INTEGRATION") != "1" {
		t.Skip("set XMDM_RUN_OBJECT_STORAGE_INTEGRATION=1 to run against live object storage")
	}
	store, err := New(context.Background(), Config{
		Endpoint:        getenv("XMDM_OBJECT_STORAGE_ENDPOINT", "http://127.0.0.1:8333"),
		Region:          getenv("XMDM_OBJECT_STORAGE_REGION", "us-east-1"),
		AccessKeyID:     getenv("XMDM_OBJECT_STORAGE_ACCESS_KEY", "xmdm"),
		SecretAccessKey: getenv("XMDM_OBJECT_STORAGE_SECRET_KEY", "xmdm"),
		Bucket:          getenv("XMDM_OBJECT_STORAGE_BUCKET", "xmdm"),
		UsePathStyle:    true,
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	key := fmt.Sprintf("integration/seaweedfs-%d.bin", time.Now().UnixNano())
	data := bytes.Repeat([]byte("z"), 128)
	if err := store.Put(context.Background(), key, bytes.NewReader(data), "application/octet-stream", int64(len(data))); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
