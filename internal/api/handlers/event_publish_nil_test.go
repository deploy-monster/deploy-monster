package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestPublishEventHelpers_NilEventBus(t *testing.T) {
	publishEvent(t.Context(), nil, core.NewEvent("test.sync", "test", nil))
	publishEventAsync(t.Context(), nil, core.NewEvent("test.async", "test", nil))
}

func TestLifecycleHandlers_NilEventBus(t *testing.T) {
	tests := []struct {
		name    string
		handler func(*mockStore) http.HandlerFunc
		method  string
		path    string
		body    any
		want    int
	}{
		{
			name: "scale",
			handler: func(store *mockStore) http.HandlerFunc {
				return NewScaleHandler(store, nil).Scale
			},
			method: http.MethodPost,
			path:   "/api/v1/apps/app1/scale",
			body:   scaleRequest{Replicas: 3},
			want:   http.StatusOK,
		},
		{
			name: "clone",
			handler: func(store *mockStore) http.HandlerFunc {
				return NewCloneHandler(store, nil).Clone
			},
			method: http.MethodPost,
			path:   "/api/v1/apps/app1/clone",
			body:   cloneRequest{NewName: "copy"},
			want:   http.StatusCreated,
		},
		{
			name: "rename",
			handler: func(store *mockStore) http.HandlerFunc {
				return NewRenameHandler(store, nil).Rename
			},
			method: http.MethodPost,
			path:   "/api/v1/apps/app1/rename",
			body:   map[string]string{"name": "Renamed"},
			want:   http.StatusOK,
		},
		{
			name: "snapshot",
			handler: func(store *mockStore) http.HandlerFunc {
				return NewSnapshotHandler(store, nil, nil).Create
			},
			method: http.MethodPost,
			path:   "/api/v1/apps/app1/snapshots",
			want:   http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockStore()
			store.addApp(&core.Application{
				ID:         "app1",
				TenantID:   "tenant1",
				Name:       "Web App",
				Type:       "service",
				SourceType: "git",
				SourceURL:  "https://example.com/repo.git",
				Replicas:   1,
				Status:     "running",
			})

			var body *bytes.Reader
			if tt.body != nil {
				data, err := json.Marshal(tt.body)
				if err != nil {
					t.Fatal(err)
				}
				body = bytes.NewReader(data)
			} else {
				body = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			req.SetPathValue("id", "app1")
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			tt.handler(store).ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tt.want, rr.Body.String())
			}
		})
	}
}
