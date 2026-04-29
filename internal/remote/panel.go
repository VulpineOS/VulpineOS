package remote

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

// ServePanel adds the web panel static files to an HTTP mux.
// The panel files are embedded in the binary at build time from
// cmd/vulpineos/panel/, which is refreshed from web/dist/ by
// `npm --prefix web run build`.
// If webFS is nil, the panel is not available and requests get a 404.
func ServePanel(mux *http.ServeMux, webFS fs.FS) {
	if webFS == nil {
		log.Println("web panel not available (no embedded files)")
		return
	}

	fileServer := http.FileServer(http.FS(webFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		// Keep reserved control paths out of the SPA fallback. If a more
		// specific handler owns the path, ServeMux routes there before this
		// catch-all handler; otherwise the request should be a real 404.
		if r.URL.Path == "/ws" || r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if file exists in the embedded FS
		if f, err := webFS.Open(strings.TrimPrefix(path, "/")); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for client-side routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	log.Println("web panel serving at /")
}

// EmbedFS is a placeholder for the embedded web panel files.
// The actual embedding happens in cmd/vulpineos/main.go with //go:embed.
var _ embed.FS // ensure embed is importable
