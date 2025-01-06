package config

import (
	"flag"
	"log"
	"os"

	"github.com/BrownBear56/contractor/internal/logger"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Address         string
	BaseURL         string
	FileStoragePath string
	DatabaseDSN     string
}

func NewConfig(parentLogger logger.Logger) *Config {
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

	configLogger, err := parentLogger.(*logger.ZapLogger).ReconfigureAndNamed(
		"Config",
		"info",             // Уровень логирования
		"json",             // Формат логов
		[]string{"stdout"}, // Вывод логов
		&customEncoderConfig,
	)
	if err != nil {
		log.Fatalf("Failed to reconfigure logger: %v", err)
	}

	// Флаги командной строки.
	addressFlag := flag.String("a", "localhost:8080", "HTTP server address.")
	baseURLFlag := flag.String("b", "http://localhost:8080", "Base URL for shortened links.")
	filePathFlag := flag.String("f", "storage.json", "Path to file storage.")
	databaseDSNFlag := flag.String("d", "", "Database connection string.")

	flag.Parse()

	// Приоритет: переменные окружения → флаги → значения по умолчанию.
	address := *addressFlag
	if envAddress, ok := os.LookupEnv("SERVER_ADDRESS"); ok {
		address = envAddress
	}

	baseURL := *baseURLFlag
	if envBaseURL, ok := os.LookupEnv("BASE_URL"); ok {
		baseURL = envBaseURL
	}

	fileStoragePath := *filePathFlag
	if envFilePath, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		fileStoragePath = envFilePath
	}

	databaseDSN := *databaseDSNFlag
	if envDSN, ok := os.LookupEnv("DATABASE_DSN"); ok {
		databaseDSN = envDSN
	}

	// Валидация базового URL.
	if baseURL == "" {
		configLogger.Info("Base URL cannot be empty. Using default value.")
		baseURL = "http://localhost:8080"
	}

	// Валидация строки подключения к БД
	if baseURL == "" {
		configLogger.Info("Connection string cannot be empty. Using default value.")
		baseURL = ""
	}

	return &Config{
		Address:         address,
		BaseURL:         baseURL,
		FileStoragePath: fileStoragePath,
		DatabaseDSN:     databaseDSN,
	}
}
