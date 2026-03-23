package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:static
var staticFS embed.FS

// spaHandler serves the React SPA from embedded files.
// Falls back to index.html for client-side routing.
type spaHandler struct {
	fileServer http.Handler
	fsys       fs.FS
}

// newSPAHandler creates an SPA handler from the embedded filesystem.
// If the embedded files don't exist (dev mode), returns a placeholder.
func newSPAHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Fallback: no embedded UI, serve placeholder
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<!DOCTYPE html><html><head><title>DeployMonster</title></head>
<body style="font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#0f172a;color:#f1f5f9">
<div style="text-align:center"><h1 style="color:#10b981">DeployMonster</h1><p>UI not embedded. Run <code>npm run build</code> in web/ first.</p></div>
</body></html>`))
		})
	}

	return &spaHandler{
		fileServer: http.FileServer(http.FS(sub)),
		fsys:       sub,
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "index.html"
	} else {
		path = strings.TrimPrefix(path, "/")
	}

	// Try to serve the exact file
	if _, err := fs.Stat(h.fsys, path); err == nil {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	// SPA fallback: serve index.html for all non-file routes
	r.URL.Path = "/"
	h.fileServer.ServeHTTP(w, r)
}
