package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/artifactcleanup"
	"xmdm/server/internal/artifacts"
	s3store "xmdm/server/internal/artifacts/s3"
	"xmdm/server/internal/bootstrap"
	commandspg "xmdm/server/internal/commands/postgres"
	"xmdm/server/internal/enrollment"
	enrollmentpg "xmdm/server/internal/enrollment/postgres"
	filespg "xmdm/server/internal/files/postgres"
)

func main() {
	tenantID := flag.String("tenant-id", bootstrap.SeedTenantID, "tenant to clean up")
	apply := flag.Bool("apply", false, "apply cleanup to stale enrollment tokens, commands, and orphaned artifacts")
	flag.Parse()

	ctx := context.Background()
	pool := mustPostgresPool(ctx)
	defer pool.Close()

	now := time.Now().UTC()
	enrollmentStore := enrollmentpg.New(pool)
	commandStore := commandspg.New(pool)
	fileStore := filespg.New(pool)
	artifactStore := mustArtifactStore(ctx)

	staleTokens, err := countExpiredEnrollmentTokens(ctx, pool, *tenantID, now)
	if err != nil {
		log.Fatal(err)
	}
	staleCommands, err := countExpiredCommands(ctx, pool, *tenantID, now)
	if err != nil {
		log.Fatal(err)
	}

	result, err := artifactcleanup.Run(ctx, fileStore, artifactStore, *tenantID, *apply)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("found %d expired enrollment token candidates for tenant %s", staleTokens, *tenantID)
	log.Printf("found %d expired command candidates for tenant %s", staleCommands, *tenantID)
	log.Printf("found %d orphan artifact candidates for tenant %s", len(result.Candidates), *tenantID)
	if !*apply {
		return
	}

	expiredTokens, err := enrollmentStore.ExpireTokens(ctx, now)
	if err != nil {
		log.Fatal(err)
	}
	expiredCommands, err := commandStore.ExpireDueCommands(ctx, *tenantID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("expired %d enrollment tokens", expiredTokens)
	log.Printf("expired %d commands", expiredCommands)
	log.Printf("retired %d artifacts and deleted %d artifact records", len(result.Retired), len(result.Deleted))
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

func countExpiredEnrollmentTokens(ctx context.Context, pool *pgxpool.Pool, tenantID string, before time.Time) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx,
		`SELECT count(*)
		 FROM enrollment_tokens
		 WHERE tenant_id = $1
		   AND status = $2
		   AND expires_at <= $3`,
		tenantID, enrollment.TokenStatusIssued, before,
	).Scan(&count)
	return count, err
}

func countExpiredCommands(ctx context.Context, pool *pgxpool.Pool, tenantID string, before time.Time) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx,
		`SELECT count(*)
		 FROM commands
		 WHERE tenant_id = $1
		   AND status IN ($2::text, $3::text)
		   AND expires_at IS NOT NULL
		   AND expires_at <= $4`,
		tenantID, "queued", "sent", before,
	).Scan(&count)
	return count, err
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
