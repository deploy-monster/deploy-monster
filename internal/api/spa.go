package api

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
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
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>DeployMonster</title></head>
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

// assetPrefixes is the set of URL prefixes that must map 1:1 to a file
// in the embedded filesystem. A miss here is a real 404 (stale bundle,
// missing chunk, wrong hash) and must NOT fall back to index.html —
var assetPrefixes = []string{
	"/assets/",
	"/chunks/",
}

// cspNoncePlaceholder is the placeholder string replaced with the
// per-request nonce in the index.html served to clients.
const cspNoncePlaceholder = "DEPLOYMONSTER"

// generateCSPNonce returns a URL-safe 16-byte random nonce encoded in base64url.
func generateCSPNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "DEPLOYMONSTER-FALLBACK"
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Root path must always be served with nonce-injected index.html for CSP
	if path == "/" {
		nonce := generateCSPNonce()
		h.serveIndexHTMLWithNonce(w, r, nonce)
		return
	}

	path = strings.TrimPrefix(path, "/")

	// Try to serve the exact file
	if _, err := fs.Stat(h.fsys, path); err == nil {
		h.serveFileWithNonce(w, r, path)
		return
	}

	// Known asset paths must not silently fall back to index.html
	for _, prefix := range assetPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
	}

	// SPA fallback: serve index.html for all non-file routes
	r.URL.Path = "/"
	nonce := generateCSPNonce()
	h.serveIndexHTMLWithNonce(w, r, nonce)
}

// serveFileWithNonce serves a static file with CSP headers that allow
// the page's nonce to satisfy script execution.
// For external script files (scripts loaded via <script src=>), the response
// CSP does NOT need a matching nonce — strict-dynamic in the page's CSP
// propagates trust to scripts loaded by a trusted script. We use a
// permissive CSP for script responses that does not require nonce matching
// on fetch, since the browser has already validated the containing page's nonce.
func (h *spaHandler) serveFileWithNonce(w http.ResponseWriter, r *http.Request, path string) {
	isScript := strings.HasSuffix(path, ".js")
	isStyle := strings.HasSuffix(path, ".css")

	if isScript {
		// Scripts loaded via strict-dynamic from an authenticated page
		// do not require a matching response nonce. We send a CSP that
		// permits execution without nonce requirement.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline' 'strict-dynamic' https:; style-src 'self' 'unsafe-inline';")
	} else if isStyle {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline';")
	} else {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self';")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")

	h.fileServer.ServeHTTP(w, r)
}

// serveIndexHTMLWithNonce serves index.html with a CSP nonce injected.
func (h *spaHandler) serveIndexHTMLWithNonce(w http.ResponseWriter, r *http.Request, nonce string) {
	data, err := fs.ReadFile(h.fsys, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	body := string(data)

	// Inject nonce into CSP meta tag: 'nonce-DEPLOYMONSTER' → 'nonce-{actual}'
	body = strings.ReplaceAll(body, "nonce-"+cspNoncePlaceholder, "nonce-"+nonce)

	// Inject nonce into module script tag if present:
	// <script type="module" crossorigin src="..."> → <script type="module" crossorigin nonce="..." src="...">
	body = strings.Replace(body,
		`<script type="module" crossorigin src=`,
		`<script type="module" crossorigin nonce="`+nonce+`" src=`,
		1,
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'strict-dynamic' 'nonce-"+nonce+"'; style-src 'self' 'nonce-"+nonce+"'; img-src 'self' data: https:; connect-src 'self' https: wss:; frame-ancestors 'none'; base-uri 'self';")
	_, _ = w.Write([]byte(body))
}
