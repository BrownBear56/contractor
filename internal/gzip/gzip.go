package gzip

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (grw *gzipResponseWriter) Write(data []byte) (int, error) {
	n, err := grw.writer.Write(data)
	if err != nil {
		return n, fmt.Errorf("gzip response write failed: %w", err)
	}
	return n, nil
}

// GzipMiddleware добавляет поддержку gzip-сжатия для входящих и исходящих данных.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Обработка входящих запросов с Content-Encoding: gzip.
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			gzipReader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Invalid gzip content", http.StatusBadRequest)
				return
			}
			defer func() {
				if err := gzipReader.Close(); err != nil {
					log.Printf("Error closing gzip reader: %v\n", err)
				}
			}()
			r.Body = io.NopCloser(gzipReader)
		}

		// Обработка исходящих ответов с Accept-Encoding: gzip.
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Создание gzip-обёртки для ответа.
		w.Header().Set("Content-Encoding", "gzip")
		gzipWriter := gzip.NewWriter(w)
		defer func() {
			if err := gzipWriter.Close(); err != nil {
				log.Printf("Error closing gzip writer: %v\n", err)
			}
		}()

		grw := &gzipResponseWriter{
			ResponseWriter: w,
			writer:         gzipWriter,
		}
		next.ServeHTTP(grw, r)
	})
}
