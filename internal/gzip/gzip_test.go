package gzip

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipMiddleware_RequestWithGzipEncoding(t *testing.T) {
	// Входящее тело
	body := []byte("test data")

	// Создание сжатого тела
	var compressedBody bytes.Buffer
	writer := gzip.NewWriter(&compressedBody)
	if _, err := writer.Write(body); err != nil {
		t.Fatalf("failed to compress body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	// Создание тестового запроса
	req := httptest.NewRequest(http.MethodPost, "/", &compressedBody)
	req.Header.Set("Content-Encoding", "gzip")

	// Обработчик для проверки разархивированных данных
	handler := GzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверка разархивированного тела
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if !bytes.Equal(bodyBytes, body) {
			t.Errorf("expected body %s, got %s", string(body), string(bodyBytes))
		}
	}))

	// Выполнение запроса
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestGzipMiddleware_ResponseWithGzipEncoding(t *testing.T) {
	// Создание тестового запроса
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Accept-Encoding", "gzip")

	// Ответ, который будет отправлен обработчиком
	expectedBody := "test response"

	// Обработчик для проверки сжатия
	handler := GzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(expectedBody)); err != nil {
			t.Fatalf("failed to write response body: %v", err)
		}
	}))

	// Выполнение запроса
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Проверка заголовков ответа
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding gzip, got %s", rec.Header().Get("Content-Encoding"))
	}

	// Проверка тела ответа
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("failed to close gzip reader: %v", err)
		}
	}()

	responseBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read compressed response body: %v", err)
	}
	if string(responseBody) != expectedBody {
		t.Errorf("expected response body %s, got %s", expectedBody, string(responseBody))
	}
}

func TestGzipMiddleware_NoGzipSupport(t *testing.T) {
	// Создание тестового запроса
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	// Ответ, который будет отправлен обработчиком
	expectedBody := "test response"

	// Обработчик для проверки отсутствия сжатия
	handler := GzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(expectedBody)); err != nil {
			t.Fatalf("failed to write response body: %v", err)
		}
	}))

	// Выполнение запроса
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Проверка отсутствия заголовка Content-Encoding
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Errorf("unexpected Content-Encoding gzip")
	}

	// Проверка тела ответа
	if rec.Body.String() != expectedBody {
		t.Errorf("expected response body %s, got %s", expectedBody, rec.Body.String())
	}
}

func TestGzipMiddleware_InvalidGzipRequest(t *testing.T) {
	// Создание тестового запроса с некорректным сжатием
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("invalid gzip data"))
	req.Header.Set("Content-Encoding", "gzip")

	// Обработчик для проверки обработки ошибки
	handler := GzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for invalid gzip data")
	}))

	// Выполнение запроса
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Проверка кода ответа
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}

	// Проверка тела ответа
	expectedError := "Invalid gzip content"
	if !strings.Contains(rec.Body.String(), expectedError) {
		t.Errorf("expected error message %s, got %s", expectedError, rec.Body.String())
	}
}
