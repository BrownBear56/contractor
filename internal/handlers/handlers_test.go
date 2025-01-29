package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/BrownBear56/contractor/internal/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestPostBatchHandler(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		expectedStatus    int
		expectedResponses []models.BatchResponse
	}{
		{
			name: "Valid batch request",
			body: `[
                {"correlation_id": "1", "original_url": "http://example.com/1"},
                {"correlation_id": "2", "original_url": "http://example.com/2"}
            ]`,
			expectedStatus: http.StatusCreated,
			expectedResponses: []models.BatchResponse{
				{CorrelationID: "1"},
				{CorrelationID: "2"},
			},
		},
		{
			name:           "Empty batch request",
			body:           `[]`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON",
			body:           `{invalid}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Duplicate URLs in batch",
			body: `[
                {"correlation_id": "1", "original_url": "http://example.com/duplicate"},
                {"correlation_id": "2", "original_url": "http://example.com/duplicate"}
            ]`,
			expectedStatus: http.StatusCreated,
			expectedResponses: []models.BatchResponse{
				{CorrelationID: "1"},
				{CorrelationID: "2"},
			},
		},
		{
			name: "Invalid URL in batch",
			body: `[
                {"correlation_id": "1", "original_url": "invalid-url"}
            ]`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	testUserID := "2d53aef9-d077-4d47-96ef-9d23fc5d2d5f"
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "storage_test.json")
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		t.Errorf("Failed to initialize logger: %v", err)
		return
	}
	defer func() {
		_ = zapLogger.Sync()
	}()

	testLogger := logger.NewZapLogger(zapLogger)

	testDBConnString := ""

	urlShortener := NewURLShortener("http://localhost:8080", filePath, testDBConnString, true, testLogger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			ctx := context.WithValue(req.Context(), UserIDKey, testUserID)
			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			urlShortener.PostBatchHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
					return
				}
			}()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "unexpected status code")

			if tt.expectedStatus == http.StatusCreated {
				bodyBytes, err := io.ReadAll(resp.Body)
				assert.NoError(t, err, "failed to read response body")

				var actualResponses []models.BatchResponse
				err = json.Unmarshal(bodyBytes, &actualResponses)
				assert.NoError(t, err, "failed to decode response JSON")

				// Проверяем количество ответов
				assert.Equal(t, len(tt.expectedResponses), len(actualResponses), "unexpected response count")

				// Проверяем соответствие CorrelationID и общий формат ShortURL
				for i, expected := range tt.expectedResponses {
					actual := actualResponses[i]
					assert.Equal(t, expected.CorrelationID, actual.CorrelationID, "unexpected CorrelationID")
					assert.Contains(t, actual.ShortURL, "http://localhost:8080/", "unexpected ShortURL format")
				}
			}
		})
	}
}

func TestPostJSONHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedPrefix string
	}{
		{
			name:           "Valid URL",
			body:           `{"url": "http://example.com"}`,
			expectedStatus: http.StatusCreated,
			expectedPrefix: "http://localhost:8080/",
		},
		{
			name:           "Empty body",
			body:           "",
			expectedStatus: http.StatusBadRequest,
			expectedPrefix: "",
		},
		{
			name:           "Invalid JSON",
			body:           "{url: http://example.com}",
			expectedStatus: http.StatusBadRequest,
			expectedPrefix: "",
		},
		{
			name:           "Duplicate URL",
			body:           `{"url": "http://example.com"}`,
			expectedStatus: http.StatusConflict,
			expectedPrefix: "http://localhost:8080/",
		},
		{
			name:           "Invalid URL",
			body:           `{"url": "not-a-url"}`,
			expectedStatus: http.StatusBadRequest,
			expectedPrefix: "",
		},
	}

	testUserID := "2d53aef9-d077-4d47-96ef-9d23fc5d2d5f"
	// Создаём временную директорию для теста.
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "storage_test.json")
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		t.Errorf("Failed to initialize logger: %v", err)
		return
	}
	defer func() {
		_ = zapLogger.Sync()
	}()

	testLogger := logger.NewZapLogger(zapLogger)

	testDBConnString := ""

	// Устанавливаем базовый URL для тестов.
	urlShortener := NewURLShortener("http://localhost:8080", filePath, testDBConnString, true, testLogger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			ctx := context.WithValue(req.Context(), UserIDKey, testUserID)
			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			urlShortener.PostJSONHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
					return
				}
			}()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "unexpected status code")

			if tt.expectedStatus == http.StatusCreated || tt.expectedStatus == http.StatusOK {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("failed to read response body: %v", err)
					return
				}

				var response models.Response
				if err := json.Unmarshal(bodyBytes, &response); err != nil {
					t.Errorf("failed to decode response JSON: %v", err)
					return
				}

				assert.True(t, strings.HasPrefix(response.Result, tt.expectedPrefix),
					"expected response to start with %q, got %q", tt.expectedPrefix, response.Result)
			}
		})
	}
}

func TestPostHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedPrefix string
	}{
		{
			name:           "Valid URL",
			body:           "http://example.com",
			expectedStatus: http.StatusCreated,
			expectedPrefix: "http://localhost:8080/",
		},
		{
			name:           "Empty body",
			body:           "",
			expectedStatus: http.StatusBadRequest,
			expectedPrefix: "",
		},
		{
			name:           "Whitespace body",
			body:           "   ",
			expectedStatus: http.StatusBadRequest,
			expectedPrefix: "",
		},
		{
			name:           "Duplicate URL",
			body:           "http://example.com",
			expectedStatus: http.StatusConflict,
			expectedPrefix: "http://localhost:8080/",
		},
		{
			name:           "Invalid URL",
			body:           "not-a-url",
			expectedStatus: http.StatusBadRequest,
			expectedPrefix: "",
		},
	}

	testUserID := "2d53aef9-d077-4d47-96ef-9d23fc5d2d5f"
	// Создаём временную директорию для теста.
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "storage_test.json")
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		t.Errorf("Failed to initialize logger: %v", err)
		return
	}
	defer func() {
		_ = zapLogger.Sync()
	}()

	testLogger := logger.NewZapLogger(zapLogger)

	testDBConnString := ""

	// Устанавливаем базовый URL для тестов.
	urlShortener := NewURLShortener("http://localhost:8080", filePath, testDBConnString, true, testLogger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			ctx := context.WithValue(req.Context(), UserIDKey, testUserID)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			urlShortener.PostHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
					return
				}
			}()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "unexpected status code")

			if tt.expectedStatus == http.StatusCreated {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("failed to read response body: %v", err)
					return
				}

				body := string(bodyBytes)
				assert.True(t, strings.HasPrefix(body, tt.expectedPrefix),
					"expected response to start with %q, got %q", tt.expectedPrefix, body)
			}
		})
	}
}

func TestGetUserURLsHandler(t *testing.T) {
	testUserID := "2d53aef9-d077-4d47-96ef-9d23fc5d2d5f"
	testShortID := "testID"
	testURL := "http://example.com"

	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		t.Errorf("Failed to initialize logger: %v", err)
		return
	}
	defer func() {
		_ = zapLogger.Sync()
	}()

	testLogger := logger.NewZapLogger(zapLogger)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "storage_test.json")
	testDBConnString := ""

	urlShortener := NewURLShortener("http://localhost:8080", filePath, testDBConnString, true, testLogger)

	if err := urlShortener.storage.SaveID(testUserID, testShortID, testURL); err != nil {
		t.Errorf("Failed to save url in storage: %v", err)
		return
	}

	tests := []struct {
		name           string
		userID         string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Valid User ID",
			userID:         testUserID,
			expectedStatus: http.StatusOK,
			expectedBody:   fmt.Sprintf(`[{"short_url":"http://localhost:8080/%s","original_url":%q}]`, testShortID, testURL),
		},
		{
			name:           "Unauthorized - Missing User ID",
			userID:         "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/user/urls", http.NoBody)

			// Добавляем userID в контекст
			ctx := context.WithValue(req.Context(), UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			urlShortener.GetUserURLsHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
					return
				}
			}()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "Unexpected status code")

			if tt.expectedStatus == http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				assert.JSONEq(t, tt.expectedBody, string(body), "Unexpected response body")
			}
		})
	}
}

func TestGetHandler(t *testing.T) {
	testUserID := "2d53aef9-d077-4d47-96ef-9d23fc5d2d5f"
	testID := "testID"
	testURL := "http://example.com"
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		t.Errorf("Failed to initialize logger: %v", err)
		return
	}
	defer func() {
		_ = zapLogger.Sync()
	}()

	testLogger := logger.NewZapLogger(zapLogger)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "storage_test.json")

	testDBConnString := ""

	urlShortener := NewURLShortener("http://localhost:8080", filePath, testDBConnString, true, testLogger)
	if err := urlShortener.storage.SaveID(testUserID, testID, testURL); err != nil {
		t.Errorf("Failed to save url in memory: %v", err)
		return
	}

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedHeader string
	}{
		{
			name:           "Valid ID",
			path:           "/" + testID,
			expectedStatus: http.StatusTemporaryRedirect,
			expectedHeader: testURL,
		},
		{
			name:           "Invalid ID",
			path:           "/invalidID",
			expectedStatus: http.StatusBadRequest,
			expectedHeader: "",
		},
		{
			name:           "Empty ID",
			path:           "/",
			expectedStatus: http.StatusBadRequest,
			expectedHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, http.NoBody)
			w := httptest.NewRecorder()

			urlShortener.GetHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
					return
				}
			}()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "unexpected status code")

			if tt.expectedHeader != "" {
				location := resp.Header.Get("Location")
				assert.Equal(t, tt.expectedHeader, location,
					"expected Location header %q, got %q", tt.expectedHeader, location)
			}
		})
	}
}

// Тесты для конкурентного использования.
func TestConcurrentAccess(t *testing.T) {
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		t.Errorf("Failed to initialize logger: %v", err)
		return
	}
	defer func() {
		_ = zapLogger.Sync()
	}()

	testUserID := "2d53aef9-d077-4d47-96ef-9d23fc5d2d5f"
	testLogger := logger.NewZapLogger(zapLogger)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "storage_test.json")

	testDBConnString := ""

	urlShortener := NewURLShortener("http://localhost:8080", filePath, testDBConnString, true, testLogger)

	var wg sync.WaitGroup
	const goroutines = 100
	url := "http://example.com"

	wg.Add(goroutines)
	goroutineIndices := make([]int, goroutines)
	for range goroutineIndices {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url))
			ctx := context.WithValue(req.Context(), UserIDKey, testUserID)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			urlShortener.PostHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
					return
				}
			}()

			// Статус может быть 201 или 200.
			assert.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict)
		}()
	}
	wg.Wait()

	// Проверяем, что URL был сохранен только один раз.
	id, exists := urlShortener.storage.GetIDByURL(url)
	assert.True(t, exists, "expected URL to be saved")
	assert.NotEmpty(t, id, "expected non-empty ID")
}
