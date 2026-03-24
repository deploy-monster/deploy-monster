package handlers

import (
	"net/http"
)

// OpenAPIHandler serves the OpenAPI specification.
type OpenAPIHandler struct {
	version string
}

func NewOpenAPIHandler(version string) *OpenAPIHandler {
	return &OpenAPIHandler{version: version}
}

// Spec handles GET /api/v1/openapi.json
func (h *OpenAPIHandler) Spec(w http.ResponseWriter, _ *http.Request) {
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "DeployMonster API",
			"description": "Self-hosted PaaS API — deploy, manage, and scale applications",
			"version":     h.version,
			"contact": map[string]string{
				"name":  "ECOSTACK TECHNOLOGY",
				"url":   "https://deploy.monster",
				"email": "api@deploy.monster",
			},
			"license": map[string]string{
				"name": "AGPL-3.0",
				"url":  "https://www.gnu.org/licenses/agpl-3.0.html",
			},
		},
		"servers": []map[string]string{
			{"url": "/api/v1", "description": "Current server"},
		},
		"paths": generatePaths(),
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]string{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
				"apiKey": map[string]any{
					"type": "apiKey",
					"in":   "header",
					"name": "X-API-Key",
				},
			},
		},
	}

	writeJSON(w, http.StatusOK, spec)
}

func generatePaths() map[string]any {
	paths := map[string]any{
		"/auth/login":    pathDef("POST", "Login", "Authenticate with email and password", false),
		"/auth/register": pathDef("POST", "Register", "Create new account", false),
		"/auth/refresh":  pathDef("POST", "Refresh", "Refresh access token", false),
		"/auth/me":       pathDef("GET", "Profile", "Get current user", true),
		"/apps":          pathDef("GET", "List Apps", "List all applications", true),
		"/apps/{id}":     pathDef("GET", "Get App", "Get application details", true),
		"/health":        pathDef("GET", "Health", "Health check", false),
		"/marketplace":   pathDef("GET", "Marketplace", "List marketplace templates", false),
		"/billing/plans": pathDef("GET", "Plans", "List billing plans", false),
		"/branding":      pathDef("GET", "Branding", "Get platform branding", false),
	}
	return paths
}

func pathDef(method, summary, desc string, auth bool) map[string]any {
	op := map[string]any{
		"summary":     summary,
		"description": desc,
		"responses": map[string]any{
			"200": map[string]string{"description": "Success"},
		},
	}
	if auth {
		op["security"] = []map[string][]string{{"bearerAuth": {}}}
	}

	m := method
	if m == "GET" {
		m = "get"
	} else {
		m = "post"
	}

	return map[string]any{m: op}
}
