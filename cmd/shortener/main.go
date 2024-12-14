package main

import (
	"log"

	"github.com/BrownBear56/contractor/internal/config"
	"github.com/BrownBear56/contractor/internal/server"
)

func main() {
	cfg := config.NewConfig()

	srv := server.New(cfg)

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
