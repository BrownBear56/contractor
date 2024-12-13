package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			expectedStatus: http.StatusOK,
			expectedPrefix: "http://localhost:8080/",
		},
		{
			name:           "Invalid URL",
			body:           "not-a-url",
			expectedStatus: http.StatusBadRequest,
			expectedPrefix: "",
		},
	}

	// Устанавливаем базовый URL для тестов.
	urlShortener := NewURLShortener("http://localhost:8080")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			urlShortener.PostHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
				}
			}()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "unexpected status code")

			if tt.expectedStatus == http.StatusCreated {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("failed to read response body: %v", err)
				}

				body := string(bodyBytes)
				assert.True(t, strings.HasPrefix(body, tt.expectedPrefix),
					"expected response to start with %q, got %q", tt.expectedPrefix, body)
			}
		})
	}
}

func TestGetHandler(t *testing.T) {
	testID := "testID"
	testURL := "http://example.com"

	urlShortener := NewURLShortener("http://localhost:8080")
	urlShortener.storage.urlStore[testID] = testURL

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
	urlShortener := NewURLShortener("http://localhost:8080")

	var wg sync.WaitGroup
	const goroutines = 100
	url := "http://example.com"

	wg.Add(goroutines)
	goroutineIndices := make([]int, goroutines)
	for range goroutineIndices {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url))
			w := httptest.NewRecorder()

			urlShortener.PostHandler(w, req)

			resp := w.Result()
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("failed to close response body: %v", err)
				}
			}()

			// Статус может быть 201 или 200.
			assert.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK)
		}()
	}
	wg.Wait()

	// Проверяем, что URL был сохранен только один раз.
	id, exists := urlShortener.storage.getIDByURL(url)
	assert.True(t, exists, "expected URL to be saved")
	assert.NotEmpty(t, id, "expected non-empty ID")
}
