package main

import (
	"log"

	"github.com/BrownBear56/contractor/internal/config"
	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/BrownBear56/contractor/internal/server"
	"go.uber.org/zap"
)

func main() {
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if err := zapLogger.Sync(); err != nil {
			log.Fatalf("Failed to sync zap logger: %v", err)
		}
	}()

	appLogger := logger.NewZapLogger(zapLogger)

	cfg := config.NewConfig(appLogger)

	srv := server.New(cfg, appLogger)

	if err := srv.Start(); err != nil {
		appLogger.Error("Server failed to start", zap.Error(err))
	}
}
