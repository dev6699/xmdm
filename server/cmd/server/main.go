package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xmdm/server/internal/admin"
	v1 "xmdm/server/internal/api/v1"
	"xmdm/server/internal/audit"
	auditpg "xmdm/server/internal/audit/postgres"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/bootstrap"
	devicepg "xmdm/server/internal/device/postgres"
	enrollment "xmdm/server/internal/enrollment"
	enrollmentpg "xmdm/server/internal/enrollment/postgres"
	grouppg "xmdm/server/internal/group/postgres"
	identitypg "xmdm/server/internal/identity/postgres"
	"xmdm/server/internal/plugins"
	policypg "xmdm/server/internal/policy/postgres"
)

func main() {
	addr := env("XMDM_ADDR", ":8080")
	username := env("XMDM_ADMIN_USERNAME", bootstrap.DefaultAdminUsername)
	password := env("XMDM_ADMIN_PASSWORD", bootstrap.DefaultAdminPassword)
	sessionTTL := envDuration("XMDM_SESSION_TTL", 24*time.Hour)

	svc := auth.NewService(username, password, sessionTTL)
	store, enrollmentStore, auditStore := openStores()
	pluginManager := plugins.Disabled()
	mux := newMux(svc, store, enrollmentStore, auditStore, pluginManager)

	log.Printf("xmdm server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func newMux(svc *auth.Service, store admin.Repository, enrollmentStore enrollment.Repository, auditStore audit.Store, pluginManager *plugins.Manager) http.Handler {
	return v1.NewMux(svc, store, enrollmentStore, auditStore, pluginManager, bootstrap.SeedTenantID)
}

func openStores() (admin.Repository, enrollment.Repository, audit.Store) {
	dsn := env("XMDM_POSTGRES_DSN", bootstrap.DefaultPostgresDSN)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	return admin.NewRepository(
		identitypg.New(pool),
		grouppg.New(pool),
		policypg.New(pool),
		devicepg.New(pool),
	), enrollmentpg.New(pool), auditpg.NewDBStore(pool)
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
