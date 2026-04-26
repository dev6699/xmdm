package main

import (
	"flag"
	"log"
	"net/http"

	"xmdm/server/internal/api/v1"
	"xmdm/server/internal/auth"
	"xmdm/server/internal/config"
)

func main() {
	configPath := flag.String("config", "", "Path to YAML configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	username := cfg.Admin.Username
	password := cfg.Admin.Password
	sessionTTL := cfg.Server.SessionTTL

	svc := auth.NewService(username, password, sessionTTL)
	deps := v1.NewDeps(cfg)
	mux := v1.NewMux(svc, deps)

	addr := cfg.Server.Address
	log.Printf("xmdm server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
