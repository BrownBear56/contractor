package config

import (
	"flag"
	"log"
	"os"
)

type Config struct {
	Address string
	BaseURL string
}

func NewConfig() *Config {
	// Флаги командной строки.
	addressFlag := flag.String("a", "localhost:8080", "HTTP server address.")
	baseURLFlag := flag.String("b", "http://localhost:8080", "Base URL for shortened links.")

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

	// Валидация базового URL.
	if baseURL == "" {
		log.Println("Base URL cannot be empty. Using default value.")
		baseURL = "http://localhost:8080"
	}

	return &Config{
		Address: address,
		BaseURL: baseURL,
	}
}
