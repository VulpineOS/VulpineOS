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
