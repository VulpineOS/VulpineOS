package remote

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestServePanelFallsBackToIndexForSPARoutes(t *testing.T) {
	mux := http.NewServeMux()
	ServePanel(mux, fstest.MapFS{
		"index.html": {Data: []byte("<html>panel</html>")},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agents/agent-1", nil)
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "<html>panel</html>" {
		t.Fatalf("body = %q, want embedded index", string(body))
	}
}

func TestServePanelSetsSecurityHeaders(t *testing.T) {
	mux := http.NewServeMux()
	ServePanel(mux, fstest.MapFS{
		"index.html": {Data: []byte("<html>panel</html>")},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mux.ServeHTTP(resp, req)

	for name, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	} {
		if got := resp.Header().Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestServePanelDoesNotSwallowReservedControlPaths(t *testing.T) {
	for _, path := range []string{"/api/missing", "/ws", "/health"} {
		t.Run(path, func(t *testing.T) {
			mux := http.NewServeMux()
			ServePanel(mux, fstest.MapFS{
				"index.html": {Data: []byte("<html>panel</html>")},
			})

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			mux.ServeHTTP(resp, req)

			if resp.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
			}
		})
	}
}
