package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
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
	}

	// Устанавливаем базовый URL для тестов
	InitHandlers("http://localhost:8080")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()

			PostHandler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "unexpected status code")

			if tt.expectedStatus == http.StatusCreated {
				body := w.Body.String()
				assert.True(t, strings.HasPrefix(body, tt.expectedPrefix),
					"expected response to start with %q, got %q", tt.expectedPrefix, body)
			}
		})
	}
}

func TestGetHandler(t *testing.T) {
	testID := "testID"
	testURL := "http://example.com"
	mu.Lock()
	urlStore[testID] = testURL
	mu.Unlock()

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
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			GetHandler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "unexpected status code")

			if tt.expectedHeader != "" {
				location := resp.Header.Get("Location")

				assert.Equal(t, tt.expectedHeader, location,
					"expected Location header %q, got %q", tt.expectedHeader, location)
			}
		})
	}
}
