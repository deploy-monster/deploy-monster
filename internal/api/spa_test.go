package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// TestSPAHandler_AssetMissReturns404 guards the Tier 102 fix: a missing
// /assets/ or /chunks/ request must NOT fall back to index.html, or the
// browser receives HTML-as-JS and every lazy-loaded page hangs on
// Suspense forever. Regression test for the E2E Playwright suite that
// flat-lined on "status Loading" for 77/86 tests.
func TestSPAHandler_AssetMissReturns404(t *testing.T) {
	h := newSPAHandler()

	cases := []string{
		"/assets/does-not-exist.js",
		"/assets/Login-deadbeef.js",
		"/chunks/Register-cafed00d.js",
		"/chunks/vendor-react-xxxxxx.js",
	}

	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404 (asset miss must not fall back to index.html)", rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
				t.Errorf("content-type = %q, want non-HTML for asset miss", ct)
			}
		})
	}
}

// TestSPAHandler_NonAssetFallbackToIndex verifies the SPA fallback
// still works for client-routed paths like /login, /register, /apps/123.
// These are NOT real files, so the handler should serve index.html so
// React Router can take over on the client. Only /assets/ and /chunks/
// are special-cased to 404.
func TestSPAHandler_NonAssetFallbackToIndex(t *testing.T) {
	h := newSPAHandler()

	cases := []string{
		"/",
		"/login",
		"/register",
		"/apps/abc123",
		"/dashboard",
		"/settings/profile",
	}

	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			// Either 200 (embed present) or 200 from placeholder HTML
			// (embed missing in dev). A 404 here would mean the SPA
			// fallback is broken.
			if rr.Code == http.StatusNotFound {
				t.Errorf("status = 404 for %q, want SPA fallback", path)
			}
		})
	}
}

// TestEmbedIndexHTMLReferencesResolve is the strongest invariant the
// server can assert about the embedded React bundle: every /assets/
// and /chunks/ path referenced by static/index.html (via <script src>
// or <link href>) MUST resolve to a real file in the embedded FS.
//
// This is the exact failure mode from Tier 102: vite moved lazy-page
// chunks to /chunks/, but a stale commit left index.html pointing at
// /assets/ chunks that no longer existed, silently 404-ing the entire
// SPA bundle (masked by the old spa.go fallback). Running this test
// every CI catches that class of regression at build time instead of
// discovering it through 77 mysteriously-hung Playwright tests.
func TestEmbedIndexHTMLReferencesResolve(t *testing.T) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub failed: %v", err)
	}

	indexBytes, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Skipf("static/index.html not embedded (dev mode): %v", err)
	}
	index := string(indexBytes)

	// Match /assets/... and /chunks/... in src="..." and href="..."
	refRE := regexp.MustCompile(`(?:src|href)="(/(?:assets|chunks)/[^"]+)"`)
	matches := refRE.FindAllStringSubmatch(index, -1)
	if len(matches) == 0 {
		t.Fatal("no /assets/ or /chunks/ references found in index.html — regex broken or embed corrupt")
	}

	for _, m := range matches {
		ref := m[1]
		path := strings.TrimPrefix(ref, "/")
		t.Run(ref, func(t *testing.T) {
			if _, err := fs.Stat(sub, path); err != nil {
				t.Errorf("index.html references %q but file is missing from embed: %v", ref, err)
			}
		})
	}
}

// TestSPAHandler_ServesReferencedChunksAsJS walks the embed's /chunks/
// directory and asserts that the SPA handler serves every chunk with a
// 200 status and a JavaScript content-type. A 200 with text/html would
// mean the SPA fallback swallowed the request; any non-200 means the
// embed is corrupt. Either way the browser's dynamic import() hangs and
// every lazy-loaded React page locks on the Suspense fallback forever.
func TestSPAHandler_ServesReferencedChunksAsJS(t *testing.T) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub failed: %v", err)
	}

	entries, err := fs.ReadDir(sub, "chunks")
	if err != nil {
		t.Skipf("static/chunks not embedded (dev mode): %v", err)
	}

	var jsFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			jsFiles = append(jsFiles, e.Name())
		}
	}
	if len(jsFiles) == 0 {
		t.Skip("no .js chunks in embed — skipping")
	}

	h := newSPAHandler()
	// Sample at most 5 chunks to keep the test fast but representative.
	sample := jsFiles
	if len(sample) > 5 {
		sample = sample[:5]
	}
	for _, name := range sample {
		path := "/chunks/" + name
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 for existing chunk %q", rr.Code, path)
			}
			ct := rr.Header().Get("Content-Type")
			if strings.HasPrefix(ct, "text/html") {
				t.Errorf("content-type = %q for %q — SPA fallback is swallowing chunk requests", ct, path)
			}
		})
	}
}
