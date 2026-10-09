package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPRoutesForwardToAPIGateway(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "protected resource metadata", method: http.MethodGet, path: "/.well-known/oauth-protected-resource"},
		{name: "mcp endpoint", method: http.MethodPost, path: "/mcp"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != testCase.path {
					t.Fatalf("path = %q, want %q", request.URL.Path, testCase.path)
				}
				if request.Method != testCase.method {
					t.Fatalf("method = %q, want %q", request.Method, testCase.method)
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			handler := newHandler(t.TempDir(), mustProxy(upstream.URL), http.NotFoundHandler())
			request := httptest.NewRequest(testCase.method, testCase.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent {
				body, _ := io.ReadAll(response.Result().Body)
				t.Fatalf("status = %d, want %d; body = %q", response.Code, http.StatusNoContent, body)
			}
		})
	}
}