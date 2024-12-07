package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeRequest(t *testing.T, handler http.HandlerFunc, method, path, body string) (
	*httptest.ResponseRecorder, func()) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	// Возвращаем функцию для закрытия тела
	return w, func() {
		if err := w.Result().Body.Close(); err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
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
			resp, closeBody := makeRequest(t, urlShortener.PostHandler, http.MethodPost, "/", tt.body)
			defer closeBody()

			assert.Equal(t, tt.expectedStatus, resp.Result().StatusCode, "unexpected status code")

			if tt.expectedStatus == http.StatusCreated {
				body := resp.Body.String()
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
	urlShortener.storage.save(testID, testURL)

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
			resp, closeBody := makeRequest(t, urlShortener.GetHandler, http.MethodGet, tt.path, "")
			defer closeBody()
			assert.Equal(t, tt.expectedStatus, resp.Result().StatusCode, "unexpected status code")

			if tt.expectedHeader != "" {
				location := resp.Result().Header.Get("Location")
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
			resp, closeBody := makeRequest(t, urlShortener.PostHandler, http.MethodPost, "/", url)
			defer closeBody()
			// Статус может быть 201 или 200.
			assert.True(t, resp.Result().StatusCode == http.StatusCreated || resp.Result().StatusCode == http.StatusOK)
		}()
	}
	wg.Wait()

	// Проверяем, что URL был сохранен только один раз.
	id, exists := urlShortener.storage.getIDByURL(url)
	assert.True(t, exists, "expected URL to be saved")
	assert.NotEmpty(t, id, "expected non-empty ID")
}
