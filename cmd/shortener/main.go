package main

import (
	"log"

	"github.com/BrownBear56/contractor/internal/config"
	"github.com/BrownBear56/contractor/internal/server"
)

func main() {
	cfg := config.NewConfig()

	srv := server.New(cfg)

	log.Printf("Server is running on http://%s\n", cfg.Address)
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
