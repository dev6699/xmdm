package main

import (
	"flag"
	"log"

	coremigrate "xmdm/server"
	"xmdm/server/host"
	"xmdm/server/internal/config"
)

func main() {
	configPath := flag.String("config", "", "Path to YAML configuration file")
	migrateOnly := flag.Bool("migrate-only", false, "Apply core database migrations and exit")
	flag.Parse()

	if *migrateOnly {
		cfg, err := config.LoadConfig(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := coremigrate.MigrateDSN(cfg.Postgres.DSN); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := host.Run(*configPath); err != nil {
		log.Fatal(err)
	}
}
