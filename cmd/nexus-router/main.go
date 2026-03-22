package main

import (
	"flag"
	"log"
	"os"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to NexusRouter YAML config")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("-config is required")
	}

	file, err := os.Open(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	cfg, err := config.Load(file)
	if err != nil {
		log.Fatal(err)
	}

	srv, err := app.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
