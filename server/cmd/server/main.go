package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	v1 "xmdm/server/internal/api/v1"
	appspg "xmdm/server/internal/apps/postgres"
	"xmdm/server/internal/artifacts"
	s3store "xmdm/server/internal/artifacts/s3"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	certificatesspg "xmdm/server/internal/certificates/postgres"
	devicepg "xmdm/server/internal/device/postgres"
	enrollmentpg "xmdm/server/internal/enrollment/postgres"
	filespg "xmdm/server/internal/files/postgres"
	grouppg "xmdm/server/internal/group/postgres"
	identitypg "xmdm/server/internal/identity/postgres"
	"xmdm/server/internal/plugins"
	policypg "xmdm/server/internal/policy/postgres"
	telemetrypg "xmdm/server/internal/telemetry/postgres"
)

func main() {
	addr := env("XMDM_ADDR", ":8080")
	username := env("XMDM_ADMIN_USERNAME", bootstrap.DefaultAdminUsername)
	password := env("XMDM_ADMIN_PASSWORD", bootstrap.DefaultAdminPassword)
	sessionTTL := envDuration("XMDM_SESSION_TTL", 24*time.Hour)

	svc := auth.NewService(username, password, sessionTTL)
	deps := openStores()
	pluginManager := plugins.Disabled()
	deps.PluginManager = pluginManager
	mux := newMux(svc, deps)

	log.Printf("xmdm server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func newMux(svc *auth.Service, deps v1.Dependencies) http.Handler {
	return v1.NewMux(svc, deps)
}

func openStores() v1.Dependencies {
	dsn := env("XMDM_POSTGRES_DSN", bootstrap.DefaultPostgresDSN)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	artifactStore := mustArtifactStore()
	return v1.Dependencies{
		Identity:     identitypg.New(pool),
		Apps:         appspg.New(pool),
		Files:        filespg.New(pool),
		Certificates: certificatesspg.New(pool),
		Artifacts:    artifactStore,
		Groups:       grouppg.New(pool),
		Policies:     policypg.New(pool),
		Devices:      devicepg.New(pool),
		Enrollment:   enrollmentpg.New(pool),
		Telemetry:    telemetrypg.New(pool),
		Audit:        auditpg.NewDBStore(pool),
		TenantID:     bootstrap.SeedTenantID,
	}
}

func mustArtifactStore() artifacts.Store {
	endpoint := env("XMDM_OBJECT_STORAGE_ENDPOINT", "http://127.0.0.1:8333")
	region := env("XMDM_OBJECT_STORAGE_REGION", "us-east-1")
	accessKey := env("XMDM_OBJECT_STORAGE_ACCESS_KEY", "xmdm")
	secretKey := env("XMDM_OBJECT_STORAGE_SECRET_KEY", "xmdm")
	bucket := env("XMDM_OBJECT_STORAGE_BUCKET", "xmdm")
	store, err := s3store.New(context.Background(), s3store.Config{
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

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
