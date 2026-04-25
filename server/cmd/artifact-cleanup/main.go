package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/artifactcleanup"
	"xmdm/server/internal/artifacts"
	s3store "xmdm/server/internal/artifacts/s3"
	"xmdm/server/internal/bootstrap"
	filespg "xmdm/server/internal/files/postgres"
)

func main() {
	tenantID := flag.String("tenant-id", bootstrap.SeedTenantID, "tenant to scan")
	apply := flag.Bool("apply", false, "delete orphaned artifact objects and metadata")
	flag.Parse()

	ctx := context.Background()
	pool := mustPostgresPool(ctx)
	defer pool.Close()

	artifactStore := mustArtifactStore(ctx)
	fileStore := filespg.New(pool)

	result, err := artifactcleanup.Run(ctx, fileStore, artifactStore, *tenantID, *apply)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("found %d orphan artifact candidates for tenant %s", len(result.Candidates), *tenantID)
	for _, artifact := range result.Candidates {
		log.Printf("candidate id=%s status=%s storage_key=%s checksum=%s", artifact.ID, artifact.Status, artifact.StorageKey, artifact.Checksum)
	}
	if *apply {
		log.Printf("retired %d artifacts and deleted %d artifact records", len(result.Retired), len(result.Deleted))
	}
}

func mustPostgresPool(ctx context.Context) *pgxpool.Pool {
	dsn := env("XMDM_POSTGRES_DSN", bootstrap.DefaultPostgresDSN)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	return pool
}

func mustArtifactStore(ctx context.Context) artifacts.Store {
	endpoint := env("XMDM_OBJECT_STORAGE_ENDPOINT", "http://127.0.0.1:8333")
	region := env("XMDM_OBJECT_STORAGE_REGION", "us-east-1")
	accessKey := env("XMDM_OBJECT_STORAGE_ACCESS_KEY", "xmdm")
	secretKey := env("XMDM_OBJECT_STORAGE_SECRET_KEY", "xmdm")
	bucket := env("XMDM_OBJECT_STORAGE_BUCKET", "xmdm")
	store, err := s3store.New(ctx, s3store.Config{
		Endpoint:        endpoint,
		Region:          region,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		Bucket:          bucket,
		UsePathStyle:    true,
	})
	if err != nil {
		log.Fatalf("init object storage: %v", err)
	}
	return store
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
