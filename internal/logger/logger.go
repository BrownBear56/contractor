package logger

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger — интерфейс для логирования.
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Named(name string) Logger
}

// ZapLogger — обёртка для zap.Logger.
type ZapLogger struct {
	logger *zap.Logger
}

// NewZapLogger создает экземпляр ZapLogger.
func NewZapLogger(zl *zap.Logger) *ZapLogger {
	return &ZapLogger{logger: zl}
}

// Info логирует сообщение уровня INFO.
func (l *ZapLogger) Info(msg string, fields ...zap.Field) {
	l.logger.Info(msg, fields...)
}

// Error логирует сообщение уровня ERROR.
func (l *ZapLogger) Error(msg string, fields ...zap.Field) {
	l.logger.Error(msg, fields...)
}

// Named добавляет новый сегмент пути к имени регистратора.
func (l *ZapLogger) Named(name string) Logger {
	newZapLogger := l.logger.Named(name)
	return &ZapLogger{logger: newZapLogger}
}

func (l *ZapLogger) ReconfigureAndNamed(name string, level string, encoding string,
	outputPaths []string, encoderConfig zapcore.EncoderConfig) (Logger, error) {
	// Преобразуем текстовый уровень логирования в zap.AtomicLevel.
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level '%s': %w", level, err)
	}

	// Создаём новую конфигурацию логгера.
	cfg := zap.Config{
		Encoding:      encoding,
		Level:         lvl,
		OutputPaths:   outputPaths,
		EncoderConfig: encoderConfig,
	}

	// Строим новый zap.Logger на основе конфигурации.
	newLogger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger configuration: %w", err)
	}

	// Создаём новый ZapLogger и применяем Named.
	reconfiguredLogger := &ZapLogger{logger: newLogger}
	return reconfiguredLogger.Named(name), nil
}

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

func LoggingMiddleware(next http.Handler, log Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Обёртка для записи ответа с логированием.
		rr := &responseRecorder{responseWriter: w, statusCode: http.StatusOK}

		// Логируем входящий запрос
		log.Info("Incoming HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
		)

		// Выполняем следующий обработчик
		next.ServeHTTP(rr, r)

		// Логируем информацию об ответе
		log.Info("HTTP response",
			zap.Int("status", rr.statusCode),
			zap.Int("contentLength", rr.contentLength),
		)
	})
}
