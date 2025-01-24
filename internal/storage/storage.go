package storage

import (
	"log"

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/BrownBear56/contractor/internal/storage/file"
	"github.com/BrownBear56/contractor/internal/storage/memory"
	"github.com/BrownBear56/contractor/internal/storage/postgres"
	"go.uber.org/zap/zapcore"
)

type Storage interface {
	SaveID(id, originalURL string) error
	Get(id string) (string, bool)
	GetIDByURL(originalURL string) (string, bool)
	SaveBatch(pairs map[string]string) error
}

func NewStorage(filePath string, useFile bool, dbDSN string, parentLogger logger.Logger) Storage {
	// Настройки для нового логгера.
	customEncoderConfig := zapcore.EncoderConfig{
		TimeKey:       "timestamp",
		LevelKey:      "severity",
		NameKey:       "logger",
		CallerKey:     "caller",
		MessageKey:    "message",
		StacktraceKey: "stacktrace",
		EncodeTime:    zapcore.ISO8601TimeEncoder,
		EncodeLevel:   zapcore.CapitalLevelEncoder,
		EncodeCaller:  zapcore.ShortCallerEncoder,
	}

	storageLogger, err := parentLogger.(*logger.ZapLogger).ReconfigureAndNamed(
		"Storage",
		"info",             // Уровень логирования
		"json",             // Формат логов
		[]string{"stdout"}, // Вывод логов
		&customEncoderConfig,
	)
	if err != nil {
		log.Fatalf("Failed to reconfigure logger: %v", err)
	}

	if dbDSN != "" {
		pgStore, err := postgres.NewPostgresStore(dbDSN, storageLogger)
		if err != nil {
			log.Fatalf("Failed to initialize PostgresStore: %v", err)
		}
		return pgStore
	}

	if useFile {
		return file.NewFileStore(filePath, storageLogger)
	}
	return memory.NewMemoryStore()
}
