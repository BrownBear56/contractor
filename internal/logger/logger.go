package logger

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type responseRecorder struct {
	responseWriter http.ResponseWriter
	statusCode     int
	contentLength  int
}

// Header проксирует вызов к оригинальному ResponseWriter.
func (rr *responseRecorder) Header() http.Header {
	return rr.responseWriter.Header()
}

// Переопределяем метод WriteHeader для захвата статуса.
func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.responseWriter.WriteHeader(code)
}

// Переопределяем метод Write для подсчёта длины содержимого.
func (rr *responseRecorder) Write(b []byte) (int, error) {
	size, err := rr.responseWriter.Write(b)
	if err != nil {
		return size, fmt.Errorf("response write failed: %w", err)
	}
	rr.contentLength += size
	return size, nil
}

// Log будет доступен всему коду как синглтон.
// Никакой код, кроме функции Initialize, не должен модифицировать эту переменную.
// По умолчанию установлен no-op-логер, который не выводит никаких сообщений.
var Log *zap.Logger = zap.NewNop()

// Initialize инициализирует синглтон логера с необходимым уровнем логирования.
func Initialize(level string) error {
	// преобразуем текстовый уровень логирования в zap.AtomicLevel.
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return fmt.Errorf("failed to parse log level '%s': %w", level, err)
	}
	// создаём новую конфигурацию логера.
	cfg := zap.Config{
		Encoding: "json", // Логи остаются в формате JSON
		Level:    lvl,
		OutputPaths: []string{
			"stdout", // Выводим логи в консоль
		},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:       "time",                      // Ключ для времени
			LevelKey:      "level",                     // Ключ для уровня
			NameKey:       "logger",                    // Ключ для имени логгера
			CallerKey:     "caller",                    // Ключ для вызова
			MessageKey:    "msg",                       // Ключ для сообщения
			StacktraceKey: "stacktrace",                // Ключ для стектрейса
			EncodeTime:    zapcore.ISO8601TimeEncoder,  // Форматируем время в ISO 8601
			EncodeLevel:   zapcore.CapitalLevelEncoder, // Уровень логов в верхнем регистре (INFO, ERROR)
			EncodeCaller:  zapcore.ShortCallerEncoder,  // Сокращённый формат вызова
		},
	}
	// создаём логер на основе конфигурации.
	zl, err := cfg.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger configuration: %w", err)
	}
	// устанавливаем синглтон.
	Log = zl
	return nil
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Обёртка для записи ответа с логированием.
		rr := &responseRecorder{responseWriter: w, statusCode: http.StatusOK}

		// Логируем входящий запрос
		Log.Info("Incoming HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
		)

		// Выполняем следующий обработчик
		next.ServeHTTP(rr, r)

		// Логируем информацию об ответе
		Log.Info("HTTP response",
			zap.Int("status", rr.statusCode),
			zap.Int("contentLength", rr.contentLength),
		)
	})
}
