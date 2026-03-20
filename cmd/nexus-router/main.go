package main

import (
	"log"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
)

func main() {
	srv, err := app.New(config.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
