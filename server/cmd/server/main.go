package main

import (
	"flag"
	"log"

	"xmdm/server/host"
)

func main() {
	configPath := flag.String("config", "", "Path to YAML configuration file")
	flag.Parse()

	if err := host.Run(*configPath); err != nil {
		log.Fatal(err)
	}
}
