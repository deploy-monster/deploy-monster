package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouter_CrossTenantMutationMatrix closes the mutation side of the
// multi-tenant authorization matrix. FuzzRouter_CrossTenant already walks
// every resource-scoped GET with a foreign-tenant app ID; that target's
// oracle rejects any 2xx response. This test does the same for a
// representative subset of the ~40 mutation endpoints on
// `/api/v1/apps/{id}/…` — the surface where a regression isn't just a
// read leak but an actual data-mutation capability across tenants.
//
// Oracle: any 2xx response is a cross-tenant write leak. We accept any
// non-2xx (404 is the canonical answer from requireTenantApp; 400 is
// acceptable when the handler decodes the body before checking
// ownership — it still means no state was mutated).
//
// Route selection: one representative per mutation verb per handler
// family, chosen to cover the requireTenantApp code path. Keeping the
// list short keeps the test fast; the fuzz seeds below still walk every
// one of them. Update this list when a new app-scoped mutation route is
// added in router.go.
func TestRouter_CrossTenantMutationMatrix(t *testing.T) {
	r, token := fuzzSetupRouter(t)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		// App lifecycle
		{"PATCH", "/api/v1/apps/{id}", `{"name":"x"}`},
		{"DELETE", "/api/v1/apps/{id}", ""},
		{"POST", "/api/v1/apps/{id}/restart", ""},
		{"POST", "/api/v1/apps/{id}/stop", ""},
		{"POST", "/api/v1/apps/{id}/start", ""},
		{"POST", "/api/v1/apps/{id}/deploy", `{}`},
		{"POST", "/api/v1/apps/{id}/suspend", ""},
		{"POST", "/api/v1/apps/{id}/resume", ""},
		{"POST", "/api/v1/apps/{id}/rename", `{"name":"y"}`},
		{"POST", "/api/v1/apps/{id}/pin", ""},
		{"DELETE", "/api/v1/apps/{id}/pin", ""},

		// Config mutations — cover every handler that has been patched
		// with tenant ownership checks in prior sprints.
		{"PUT", "/api/v1/apps/{id}/ports", `[{"container_port":80,"protocol":"tcp"}]`},
		{"PUT", "/api/v1/apps/{id}/healthcheck", `{"type":"http"}`},
		{"PUT", "/api/v1/apps/{id}/sticky-sessions", `{"enabled":true,"cookie":"AFFINITY","max_age":3600}`},
		{"PUT", "/api/v1/apps/{id}/labels", `{}`},
		{"PUT", "/api/v1/apps/{id}/resources", `{}`},
		{"PUT", "/api/v1/apps/{id}/autoscale", `{"enabled":false}`},
		{"PUT", "/api/v1/apps/{id}/basic-auth", `{"enabled":false}`},
		{"PUT", "/api/v1/apps/{id}/error-pages", `{}`},
		{"PUT", "/api/v1/apps/{id}/response-headers", `{}`},
		{"PUT", "/api/v1/apps/{id}/maintenance", `{"enabled":false}`},
		{"PUT", "/api/v1/apps/{id}/middleware", `{}`},
		{"PUT", "/api/v1/apps/{id}/log-retention", `{"days":30}`},
		{"PUT", "/api/v1/apps/{id}/deploy-notifications", `{}`},
		{"PUT", "/api/v1/apps/{id}/env", `{"vars":{}}`},
		{"PUT", "/api/v1/apps/{id}/gpu", `{"enabled":false}`},
		{"PUT", "/api/v1/apps/{id}/restart-policy", `{}`},

		// Collection endpoints under app scope
		{"POST", "/api/v1/apps/{id}/redirects", `{"from":"/a","to":"/b"}`},
		{"POST", "/api/v1/apps/{id}/cron", `{}`},
		{"POST", "/api/v1/apps/{id}/commands", `{}`},
		{"POST", "/api/v1/apps/{id}/snapshots", `{}`},
		{"POST", "/api/v1/apps/{id}/clone", `{}`},
		{"POST", "/api/v1/apps/{id}/rollback", `{}`},
		{"POST", "/api/v1/apps/{id}/rollback-to-commit", `{}`},
		{"POST", "/api/v1/apps/{id}/scale", `{"instances":1}`},
		{"POST", "/api/v1/apps/{id}/save-template", `{}`},
		{"POST", "/api/v1/apps/{id}/env/import", `{}`},
		{"POST", "/api/v1/apps/{id}/deploy/preview", `{}`},
		{"POST", "/api/v1/apps/{id}/webhooks/rotate", ""},
		{"POST", "/api/v1/apps/{id}/webhooks/test", `{}`},

		// Admin-gated — foreign-tenant request should still not succeed.
		// Transfer is adminOnly but the role-check is not the isolation
		// barrier; requireTenantApp is.
		{"POST", "/api/v1/apps/{id}/transfer", `{"to_tenant":"tenant-C"}`},
	}

	for _, tc := range cases {
		tc := tc
		name := tc.method + " " + tc.path
		t.Run(name, func(t *testing.T) {
			path := strings.Replace(tc.path, "{id}", fuzzForeignAppID, 1)

			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, path, body)
			req.Header.Set("Authorization", "Bearer "+token)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			rr := httptest.NewRecorder()
			func() {
				defer func() { _ = recover() }()
				r.mux.ServeHTTP(rr, req)
			}()

			if rr.Code >= 200 && rr.Code < 300 {
				t.Errorf("cross-tenant MUTATION leak: %s %s returned %d (body: %s)",
					tc.method, tc.path, rr.Code, truncate(rr.Body.String(), 200))
			}
			// 401 would mean the bearer token never reached the handler —
			// that's also a misconfiguration we want to surface. A valid
			// cross-tenant probe must reach requireTenantApp and be
			// rejected with 404, not bounced at the middleware layer.
			if rr.Code == http.StatusUnauthorized {
				t.Errorf("unexpected 401 on %s %s — token didn't reach handler", tc.method, tc.path)
			}
		})
	}
}
