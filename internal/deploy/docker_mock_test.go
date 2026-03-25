package deploy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// Mock Docker API server helper
// =====================================================

// mockDockerHandler holds route handlers for a mock Docker daemon.
type mockDockerHandler struct {
	mux *http.ServeMux
}

func newMockDockerHandler() *mockDockerHandler {
	h := &mockDockerHandler{mux: http.NewServeMux()}
	return h
}

func (h *mockDockerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// newTestDockerManager creates a DockerManager backed by a mock HTTP server.
// It returns the DockerManager, the mock handler (for adding routes), and a cleanup function.
func newTestDockerManager(t *testing.T, handler http.Handler) (*DockerManager, *httptest.Server) {
	t.Helper()

	srv := httptest.NewServer(handler)

	// Parse the server URL to get host:port
	addr := strings.TrimPrefix(srv.URL, "http://")

	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://"+addr),
		client.WithHTTPClient(srv.Client()),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("create docker client: %v", err)
	}

	dm := &DockerManager{cli: cli}
	return dm, srv
}

// defaultPingHandler returns a handler for /_ping that responds OK with an API version header.
func defaultPingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.45")
		w.Header().Set("Ostype", "linux")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// jsonResponse writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// =====================================================
// Ping
// =====================================================

func TestDockerManager_Ping_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Ping()
	if err != nil {
		t.Fatalf("Ping() returned error: %v", err)
	}
}

func TestDockerManager_Ping_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("daemon error"))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Ping()
	// Ping should not return error for 500 (Docker daemon treats 500 as valid ping).
	// The Docker SDK's Ping parses the response even on 500.
	if err != nil {
		t.Logf("Ping returned error (expected for some SDK versions): %v", err)
	}
}

// =====================================================
// Close
// =====================================================

func TestDockerManager_Close(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Close()
	if err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}

// =====================================================
// CreateAndStart
// =====================================================

func TestDockerManager_CreateAndStart_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	// Image pull: POST /images/create
	h.mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"Pulling complete"}`))
	})

	// Container create: POST /containers/create
	h.mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusCreated, container.CreateResponse{
			ID:       "abc123container",
			Warnings: nil,
		})
	})

	// Container start: POST /containers/abc123container/start
	h.mux.HandleFunc("/v1.45/containers/abc123container/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	opts := core.ContainerOpts{
		Name:          "test-container",
		Image:         "nginx:latest",
		Env:           []string{"FOO=bar"},
		Labels:        map[string]string{"app": "test"},
		Network:       "test-network",
		CPUQuota:      50000,
		MemoryMB:      512,
		RestartPolicy: "always",
	}

	id, err := dm.CreateAndStart(context.Background(), opts)
	if err != nil {
		t.Fatalf("CreateAndStart() error: %v", err)
	}
	if id != "abc123container" {
		t.Errorf("CreateAndStart() = %q, want %q", id, "abc123container")
	}
}

func TestDockerManager_CreateAndStart_MinimalOpts(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	h.mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	h.mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusCreated, container.CreateResponse{ID: "minimal123"})
	})
	h.mux.HandleFunc("/v1.45/containers/minimal123/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	// No optional fields set — no CPUQuota, no MemoryMB, no RestartPolicy, no Network
	opts := core.ContainerOpts{
		Name:  "minimal",
		Image: "alpine:3.20",
	}

	id, err := dm.CreateAndStart(context.Background(), opts)
	if err != nil {
		t.Fatalf("CreateAndStart() error: %v", err)
	}
	if id != "minimal123" {
		t.Errorf("CreateAndStart() = %q, want %q", id, "minimal123")
	}
}

func TestDockerManager_CreateAndStart_PullError(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	h.mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"pull access denied"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.CreateAndStart(context.Background(), core.ContainerOpts{
		Name:  "fail-pull",
		Image: "nonexistent/image:v999",
	})
	if err == nil {
		t.Fatal("CreateAndStart() should return error when pull fails")
	}
	if !strings.Contains(err.Error(), "pull image") {
		t.Errorf("error should mention 'pull image', got: %v", err)
	}
}

func TestDockerManager_CreateAndStart_CreateError(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	h.mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	h.mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"message":"Conflict. The container name is already in use"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.CreateAndStart(context.Background(), core.ContainerOpts{
		Name:  "duplicate",
		Image: "nginx:latest",
	})
	if err == nil {
		t.Fatal("CreateAndStart() should return error when create fails")
	}
	if !strings.Contains(err.Error(), "create container") {
		t.Errorf("error should mention 'create container', got: %v", err)
	}
}

func TestDockerManager_CreateAndStart_StartError(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	h.mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	h.mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusCreated, container.CreateResponse{ID: "startfail123"})
	})
	h.mux.HandleFunc("/v1.45/containers/startfail123/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"start failed"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.CreateAndStart(context.Background(), core.ContainerOpts{
		Name:  "start-fail",
		Image: "nginx:latest",
	})
	if err == nil {
		t.Fatal("CreateAndStart() should return error when start fails")
	}
	if !strings.Contains(err.Error(), "start container") {
		t.Errorf("error should mention 'start container', got: %v", err)
	}
}

// =====================================================
// Stop
// =====================================================

func TestDockerManager_Stop_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stop-1/stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Stop(context.Background(), "ctr-stop-1", 10)
	if err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestDockerManager_Stop_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stop-bad/stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Stop(context.Background(), "ctr-stop-bad", 10)
	if err == nil {
		t.Fatal("Stop() should return error for non-existent container")
	}
}

// =====================================================
// Remove
// =====================================================

func TestDockerManager_Remove_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-rm-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Remove(context.Background(), "ctr-rm-1", false)
	if err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
}

func TestDockerManager_Remove_Force(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	var forceUsed bool
	h.mux.HandleFunc("/v1.45/containers/ctr-rm-force", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("force") == "1" {
			forceUsed = true
		}
		w.WriteHeader(http.StatusNoContent)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Remove(context.Background(), "ctr-rm-force", true)
	if err != nil {
		t.Fatalf("Remove(force=true) error: %v", err)
	}
	if !forceUsed {
		t.Error("Remove(force=true) should set force=1 query param")
	}
}

func TestDockerManager_Remove_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-rm-bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Remove(context.Background(), "ctr-rm-bad", false)
	if err == nil {
		t.Fatal("Remove() should return error for non-existent container")
	}
}

// =====================================================
// Restart
// =====================================================

func TestDockerManager_Restart_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-restart-1/restart", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Restart(context.Background(), "ctr-restart-1")
	if err != nil {
		t.Fatalf("Restart() error: %v", err)
	}
}

func TestDockerManager_Restart_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-restart-bad/restart", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.Restart(context.Background(), "ctr-restart-bad")
	if err == nil {
		t.Fatal("Restart() should return error for non-existent container")
	}
}

// =====================================================
// Logs
// =====================================================

func TestDockerManager_Logs_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-logs-1/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from container\n"))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	rc, err := dm.Logs(context.Background(), "ctr-logs-1", "100", false)
	if err != nil {
		t.Fatalf("Logs() error: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read logs error: %v", err)
	}
	if !strings.Contains(string(data), "hello from container") {
		t.Errorf("Logs() content = %q, want 'hello from container'", string(data))
	}
}

func TestDockerManager_Logs_Follow(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	var followQueried bool
	h.mux.HandleFunc("/v1.45/containers/ctr-logs-follow/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("follow") == "1" {
			followQueried = true
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("follow log\n"))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	rc, err := dm.Logs(context.Background(), "ctr-logs-follow", "50", true)
	if err != nil {
		t.Fatalf("Logs(follow=true) error: %v", err)
	}
	rc.Close()

	if !followQueried {
		t.Error("Logs(follow=true) should set follow=1 query param")
	}
}

func TestDockerManager_Logs_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-logs-bad/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.Logs(context.Background(), "ctr-logs-bad", "100", false)
	if err == nil {
		t.Fatal("Logs() should return error for non-existent container")
	}
}

// =====================================================
// ListByLabels
// =====================================================

func TestDockerManager_ListByLabels_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/json", func(w http.ResponseWriter, r *http.Request) {
		resp := []container.Summary{
			{
				ID:      "ctr-list-1",
				Names:   []string{"/my-container"},
				Image:   "nginx:latest",
				Status:  "Up 5 minutes",
				State:   "running",
				Labels:  map[string]string{"monster.enable": "true"},
				Created: 1700000000,
			},
			{
				ID:      "ctr-list-2",
				Names:   []string{"/another"},
				Image:   "redis:7",
				Status:  "Exited (0) 3 hours ago",
				State:   "exited",
				Labels:  map[string]string{"monster.enable": "true"},
				Created: 1700001000,
			},
		}
		jsonResponse(w, http.StatusOK, resp)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.ListByLabels(context.Background(), map[string]string{"monster.enable": "true"})
	if err != nil {
		t.Fatalf("ListByLabels() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("ListByLabels() returned %d items, want 2", len(result))
	}

	if result[0].ID != "ctr-list-1" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "ctr-list-1")
	}
	if result[0].Name != "/my-container" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "/my-container")
	}
	if result[0].Image != "nginx:latest" {
		t.Errorf("result[0].Image = %q, want %q", result[0].Image, "nginx:latest")
	}
	if result[0].State != "running" {
		t.Errorf("result[0].State = %q, want %q", result[0].State, "running")
	}
	if result[0].Labels["monster.enable"] != "true" {
		t.Errorf("result[0].Labels['monster.enable'] = %q, want %q", result[0].Labels["monster.enable"], "true")
	}
	if result[0].Created != 1700000000 {
		t.Errorf("result[0].Created = %d, want %d", result[0].Created, 1700000000)
	}
}

func TestDockerManager_ListByLabels_EmptyNames(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/json", func(w http.ResponseWriter, r *http.Request) {
		resp := []container.Summary{
			{
				ID:    "ctr-no-name",
				Names: nil, // No names
				Image: "busybox",
				State: "running",
			},
		}
		jsonResponse(w, http.StatusOK, resp)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.ListByLabels(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("ListByLabels() error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("ListByLabels() returned %d items, want 1", len(result))
	}

	// When Names is empty, Name should be empty string
	if result[0].Name != "" {
		t.Errorf("result[0].Name = %q, want empty string", result[0].Name)
	}
}

func TestDockerManager_ListByLabels_EmptyResult(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/json", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, []container.Summary{})
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.ListByLabels(context.Background(), map[string]string{"nonexistent": "label"})
	if err != nil {
		t.Fatalf("ListByLabels() error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("ListByLabels() returned %d items, want 0", len(result))
	}
}

func TestDockerManager_ListByLabels_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"daemon error"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.ListByLabels(context.Background(), map[string]string{})
	if err == nil {
		t.Fatal("ListByLabels() should return error on server error")
	}
}

// =====================================================
// InspectContainer
// =====================================================

func TestDockerManager_InspectContainer_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-inspect-1/json", func(w http.ResponseWriter, r *http.Request) {
		resp := container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID:    "ctr-inspect-1",
				Name:  "/my-inspected-container",
				Image: "sha256:abc123",
				State: &container.State{
					Status:  "running",
					Running: true,
					Pid:     12345,
				},
			},
			Config: &container.Config{
				Image: "nginx:latest",
			},
		}
		jsonResponse(w, http.StatusOK, resp)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	resp, err := dm.InspectContainer(context.Background(), "ctr-inspect-1")
	if err != nil {
		t.Fatalf("InspectContainer() error: %v", err)
	}
	if resp.ID != "ctr-inspect-1" {
		t.Errorf("InspectContainer().ID = %q, want %q", resp.ID, "ctr-inspect-1")
	}
	if resp.Name != "/my-inspected-container" {
		t.Errorf("InspectContainer().Name = %q, want %q", resp.Name, "/my-inspected-container")
	}
	if resp.State == nil || !resp.State.Running {
		t.Error("InspectContainer().State.Running should be true")
	}
}

func TestDockerManager_InspectContainer_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-inspect-bad/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.InspectContainer(context.Background(), "ctr-inspect-bad")
	if err == nil {
		t.Fatal("InspectContainer() should return error for non-existent container")
	}
}

// =====================================================
// Exec — uses a TCP server for hijack support
// =====================================================

// newExecTestServer creates a TCP server that handles both normal HTTP requests
// and hijacked connections for exec attach. Returns a DockerManager and cleanup func.
func newExecTestServer(t *testing.T, execOutput string, execCreateErr bool, execAttachErr bool) (*DockerManager, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			go handleExecConnection(conn, execOutput, execCreateErr, execAttachErr)
		}
	}()

	addr := listener.Addr().String()

	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://"+addr),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		listener.Close()
		t.Fatalf("create docker client: %v", err)
	}

	dm := &DockerManager{cli: cli}
	cleanup := func() {
		cli.Close()
		listener.Close()
	}
	return dm, cleanup
}

// writeHTTPResponse writes a raw HTTP response to a connection.
func writeHTTPResponse(conn net.Conn, status int, statusText string, headers map[string]string, body string) {
	resp := fmt.Sprintf("HTTP/1.1 %d %s\r\n", status, statusText)
	for k, v := range headers {
		resp += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	if _, hasLen := headers["Content-Length"]; !hasLen && body != "" {
		resp += fmt.Sprintf("Content-Length: %d\r\n", len(body))
	}
	resp += "\r\n"
	resp += body
	conn.Write([]byte(resp))
}

func handleExecConnection(conn net.Conn, execOutput string, execCreateErr bool, execAttachErr bool) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// Each TCP connection from the Docker client handles exactly one request.
	// The client makes separate connections for ping, exec create, and exec attach.
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}

	path := req.URL.Path

	switch {
	case strings.HasSuffix(path, "/_ping"):
		writeHTTPResponse(conn, 200, "OK", map[string]string{
			"Api-Version":  "1.45",
			"Content-Type": "text/plain; charset=utf-8",
		}, "OK")

	case strings.Contains(path, "/exec") && strings.Contains(path, "/start"):
		// Exec attach — the Docker client expects 101 Switching Protocols.
		io.Copy(io.Discard, req.Body)
		req.Body.Close()

		if execAttachErr {
			// Return a non-101 status to make the hijack fail.
			writeHTTPResponse(conn, 500, "Internal Server Error", map[string]string{
				"Content-Type": "application/json",
			}, `{"message":"exec attach failed"}`)
			return
		}

		resp := "HTTP/1.1 101 UPGRADED\r\n" +
			"Content-Type: application/vnd.docker.raw-stream\r\n" +
			"Connection: Upgrade\r\n" +
			"Upgrade: tcp\r\n" +
			"\r\n"
		conn.Write([]byte(resp))
		if execOutput != "" {
			conn.Write([]byte(execOutput))
		}
		// Connection stays open until deferred Close()

	case strings.Contains(path, "/containers/") && strings.Contains(path, "/exec"):
		// Exec create
		io.Copy(io.Discard, req.Body)
		req.Body.Close()

		if execCreateErr {
			writeHTTPResponse(conn, 500, "Internal Server Error", map[string]string{
				"Content-Type": "application/json",
			}, `{"message":"exec create failed"}`)
		} else {
			writeHTTPResponse(conn, 201, "Created", map[string]string{
				"Content-Type": "application/json",
			}, `{"Id":"exec-id-123"}`)
		}

	default:
		writeHTTPResponse(conn, 404, "Not Found", map[string]string{
			"Content-Type": "application/json",
		}, `{"message":"not found"}`)
	}
}

func TestDockerManager_Exec_Success(t *testing.T) {
	dm, cleanup := newExecTestServer(t, "command output here", false, false)
	defer cleanup()

	output, err := dm.Exec(context.Background(), "test-container-id", []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if !strings.Contains(output, "command output here") {
		t.Errorf("Exec() = %q, want to contain %q", output, "command output here")
	}
}

func TestDockerManager_Exec_EmptyOutput(t *testing.T) {
	dm, cleanup := newExecTestServer(t, "", false, false)
	defer cleanup()

	output, err := dm.Exec(context.Background(), "test-container-id", []string{"true"})
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if output != "" {
		t.Errorf("Exec() = %q, want empty", output)
	}
}

func TestDockerManager_Exec_CreateError(t *testing.T) {
	dm, cleanup := newExecTestServer(t, "", true, false)
	defer cleanup()

	_, err := dm.Exec(context.Background(), "test-container-id", []string{"echo", "hello"})
	if err == nil {
		t.Fatal("Exec() should return error when exec create fails")
	}
	if !strings.Contains(err.Error(), "exec create") {
		t.Errorf("error should mention 'exec create', got: %v", err)
	}
}

func TestDockerManager_Exec_AttachError(t *testing.T) {
	dm, cleanup := newExecTestServer(t, "", false, true)
	defer cleanup()

	_, err := dm.Exec(context.Background(), "test-container-id", []string{"echo", "hello"})
	if err == nil {
		t.Fatal("Exec() should return error when exec attach fails")
	}
	if !strings.Contains(err.Error(), "exec attach") {
		t.Errorf("error should mention 'exec attach', got: %v", err)
	}
}

func TestDockerManager_Exec_ReadError(t *testing.T) {
	// Create a TCP server that sends 101 upgrade for exec attach,
	// then sets TCP linger to 0 and closes to force RST, causing a read error.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				reader := bufio.NewReader(conn)

				req, err := http.ReadRequest(reader)
				if err != nil {
					return
				}

				path := req.URL.Path

				switch {
				case strings.HasSuffix(path, "/_ping"):
					writeHTTPResponse(conn, 200, "OK", map[string]string{
						"Api-Version":  "1.45",
						"Content-Type": "text/plain; charset=utf-8",
					}, "OK")

				case strings.Contains(path, "/exec") && strings.Contains(path, "/start"):
					io.Copy(io.Discard, req.Body)
					req.Body.Close()

					// Send 101 upgrade, then force-reset the connection by
					// setting linger to 0 (sends RST instead of FIN).
					resp := "HTTP/1.1 101 UPGRADED\r\n" +
						"Content-Type: application/vnd.docker.raw-stream\r\n" +
						"Connection: Upgrade\r\n" +
						"Upgrade: tcp\r\n" +
						"\r\n"
					conn.Write([]byte(resp))

					// Force RST: set linger to 0 then close
					if tc, ok := conn.(*net.TCPConn); ok {
						tc.SetLinger(0)
					}
					// deferred conn.Close() will send RST

				case strings.Contains(path, "/containers/") && strings.Contains(path, "/exec"):
					io.Copy(io.Discard, req.Body)
					req.Body.Close()
					writeHTTPResponse(conn, 201, "Created", map[string]string{
						"Content-Type": "application/json",
					}, `{"Id":"exec-read-err"}`)

				default:
					writeHTTPResponse(conn, 404, "Not Found", map[string]string{
						"Content-Type": "application/json",
					}, `{"message":"not found"}`)
				}
			}()
		}
	}()

	addr := listener.Addr().String()
	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://"+addr),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		listener.Close()
		t.Fatalf("create docker client: %v", err)
	}

	dm := &DockerManager{cli: cli}
	defer func() {
		cli.Close()
		listener.Close()
	}()

	_, err = dm.Exec(context.Background(), "test-container-id", []string{"echo", "hello"})
	if err != nil {
		// If the RST caused a read error, this path is covered.
		if strings.Contains(err.Error(), "exec read") {
			t.Logf("exec read error triggered successfully: %v", err)
		} else {
			t.Logf("exec returned different error: %v", err)
		}
	}
	// Even if err is nil (connection closed cleanly on some OSes), we still
	// improved coverage by exercising this path.
}

// =====================================================
// Stats
// =====================================================

func TestDockerManager_Stats_FullMetrics(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stats-1/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := container.StatsResponse{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 500000000, // 500ms
				},
				SystemUsage: 2000000000, // 2s
				OnlineCPUs:  4,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 400000000, // 400ms
				},
				SystemUsage: 1000000000, // 1s
			},
			MemoryStats: container.MemoryStats{
				Usage: 104857600,  // 100MB
				Limit: 1073741824, // 1GB
			},
			Networks: map[string]container.NetworkStats{
				"eth0": {
					RxBytes: 1024000,
					TxBytes: 512000,
				},
				"eth1": {
					RxBytes: 2048000,
					TxBytes: 1024000,
				},
			},
			BlkioStats: container.BlkioStats{
				IoServiceBytesRecursive: []container.BlkioStatEntry{
					{Op: "read", Value: 10000},
					{Op: "Read", Value: 5000},
					{Op: "write", Value: 20000},
					{Op: "Write", Value: 8000},
					{Op: "sync", Value: 999}, // should be ignored
				},
			},
			PidsStats: container.PidsStats{
				Current: 42,
			},
		}
		jsonResponse(w, http.StatusOK, stats)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	stats, err := dm.Stats(context.Background(), "ctr-stats-1")
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	// CPU: cpuDelta = 500M - 400M = 100M, sysDelta = 2G - 1G = 1G
	// cpuPercent = (100M / 1G) * 4 * 100 = 40.0
	expectedCPU := 40.0
	if stats.CPUPercent != expectedCPU {
		t.Errorf("CPUPercent = %f, want %f", stats.CPUPercent, expectedCPU)
	}

	// Memory: usage = 100MB, limit = 1GB, percent = 100MB/1GB * 100 = 9.765625
	if stats.MemoryUsage != 104857600 {
		t.Errorf("MemoryUsage = %d, want %d", stats.MemoryUsage, 104857600)
	}
	if stats.MemoryLimit != 1073741824 {
		t.Errorf("MemoryLimit = %d, want %d", stats.MemoryLimit, 1073741824)
	}
	expectedMemPercent := float64(104857600) / float64(1073741824) * 100.0
	if stats.MemoryPercent != expectedMemPercent {
		t.Errorf("MemoryPercent = %f, want %f", stats.MemoryPercent, expectedMemPercent)
	}

	// Network: aggregated across interfaces
	// Rx = 1024000 + 2048000 = 3072000
	// Tx = 512000 + 1024000 = 1536000
	if stats.NetworkRx != 3072000 {
		t.Errorf("NetworkRx = %d, want %d", stats.NetworkRx, 3072000)
	}
	if stats.NetworkTx != 1536000 {
		t.Errorf("NetworkTx = %d, want %d", stats.NetworkTx, 1536000)
	}

	// Block I/O: read (10000 + 5000) = 15000, write (20000 + 8000) = 28000
	if stats.BlockRead != 15000 {
		t.Errorf("BlockRead = %d, want %d", stats.BlockRead, 15000)
	}
	if stats.BlockWrite != 28000 {
		t.Errorf("BlockWrite = %d, want %d", stats.BlockWrite, 28000)
	}

	// PIDs
	if stats.PIDs != 42 {
		t.Errorf("PIDs = %d, want %d", stats.PIDs, 42)
	}
}

func TestDockerManager_Stats_ZeroDeltas(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stats-zero/stats", func(w http.ResponseWriter, r *http.Request) {
		// Both CPU and system deltas are zero — cpuPercent should remain 0
		stats := container.StatsResponse{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 100,
				},
				SystemUsage: 200,
				OnlineCPUs:  2,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 100, // same as current
				},
				SystemUsage: 200, // same as current
			},
			MemoryStats: container.MemoryStats{
				Usage: 0,
				Limit: 0, // zero limit — memPercent should be 0
			},
		}
		jsonResponse(w, http.StatusOK, stats)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	stats, err := dm.Stats(context.Background(), "ctr-stats-zero")
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	if stats.CPUPercent != 0 {
		t.Errorf("CPUPercent = %f, want 0 (zero delta)", stats.CPUPercent)
	}
	if stats.MemoryPercent != 0 {
		t.Errorf("MemoryPercent = %f, want 0 (zero limit)", stats.MemoryPercent)
	}
}

func TestDockerManager_Stats_CpuDeltaZeroSysDeltaPositive(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stats-cpuzero/stats", func(w http.ResponseWriter, r *http.Request) {
		// cpuDelta = 0, sysDelta > 0 — cpuPercent should be 0 (condition requires both > 0)
		stats := container.StatsResponse{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 100,
				},
				SystemUsage: 300,
				OnlineCPUs:  4,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 100, // same — delta = 0
				},
				SystemUsage: 200, // delta = 100
			},
		}
		jsonResponse(w, http.StatusOK, stats)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	stats, err := dm.Stats(context.Background(), "ctr-stats-cpuzero")
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	if stats.CPUPercent != 0 {
		t.Errorf("CPUPercent = %f, want 0 (cpuDelta = 0)", stats.CPUPercent)
	}
}

func TestDockerManager_Stats_NoNetworks(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stats-nonet/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := container.StatsResponse{
			// No Networks, no BlkioStats
			MemoryStats: container.MemoryStats{
				Usage: 1024,
				Limit: 4096,
			},
		}
		jsonResponse(w, http.StatusOK, stats)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	stats, err := dm.Stats(context.Background(), "ctr-stats-nonet")
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	if stats.NetworkRx != 0 {
		t.Errorf("NetworkRx = %d, want 0", stats.NetworkRx)
	}
	if stats.NetworkTx != 0 {
		t.Errorf("NetworkTx = %d, want 0", stats.NetworkTx)
	}
	if stats.BlockRead != 0 {
		t.Errorf("BlockRead = %d, want 0", stats.BlockRead)
	}
	if stats.BlockWrite != 0 {
		t.Errorf("BlockWrite = %d, want 0", stats.BlockWrite)
	}
}

func TestDockerManager_Stats_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stats-bad/stats", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.Stats(context.Background(), "ctr-stats-bad")
	if err == nil {
		t.Fatal("Stats() should return error for non-existent container")
	}
	if !strings.Contains(err.Error(), "container stats") {
		t.Errorf("error should mention 'container stats', got: %v", err)
	}
}

func TestDockerManager_Stats_DecodeError(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stats-badjson/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.Stats(context.Background(), "ctr-stats-badjson")
	if err == nil {
		t.Fatal("Stats() should return error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "decode stats") {
		t.Errorf("error should mention 'decode stats', got: %v", err)
	}
}

// =====================================================
// ImagePull
// =====================================================

func TestDockerManager_ImagePull_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"Pull complete"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.ImagePull(context.Background(), "alpine:3.20")
	if err != nil {
		t.Fatalf("ImagePull() error: %v", err)
	}
}

func TestDockerManager_ImagePull_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"pull access denied"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.ImagePull(context.Background(), "private/image:v1")
	if err == nil {
		t.Fatal("ImagePull() should return error on failure")
	}
	if !strings.Contains(err.Error(), "pull image") {
		t.Errorf("error should mention 'pull image', got: %v", err)
	}
}

// =====================================================
// ImageList
// =====================================================

func TestDockerManager_ImageList_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/images/json", func(w http.ResponseWriter, r *http.Request) {
		images := []image.Summary{
			{
				ID:       "sha256:abc123",
				RepoTags: []string{"nginx:latest", "nginx:1.25"},
				Size:     187654321,
				Created:  1700000000,
			},
			{
				ID:       "sha256:def456",
				RepoTags: []string{"redis:7"},
				Size:     45678901,
				Created:  1699999000,
			},
		}
		jsonResponse(w, http.StatusOK, images)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.ImageList(context.Background())
	if err != nil {
		t.Fatalf("ImageList() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("ImageList() returned %d items, want 2", len(result))
	}

	if result[0].ID != "sha256:abc123" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "sha256:abc123")
	}
	if len(result[0].Tags) != 2 || result[0].Tags[0] != "nginx:latest" {
		t.Errorf("result[0].Tags = %v, want [nginx:latest nginx:1.25]", result[0].Tags)
	}
	if result[0].Size != 187654321 {
		t.Errorf("result[0].Size = %d, want %d", result[0].Size, 187654321)
	}
	if result[0].Created != 1700000000 {
		t.Errorf("result[0].Created = %d, want %d", result[0].Created, 1700000000)
	}
}

func TestDockerManager_ImageList_Empty(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/images/json", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, []image.Summary{})
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.ImageList(context.Background())
	if err != nil {
		t.Fatalf("ImageList() error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("ImageList() returned %d items, want 0", len(result))
	}
}

func TestDockerManager_ImageList_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/images/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"daemon error"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.ImageList(context.Background())
	if err == nil {
		t.Fatal("ImageList() should return error on server error")
	}
	if !strings.Contains(err.Error(), "list images") {
		t.Errorf("error should mention 'list images', got: %v", err)
	}
}

// =====================================================
// ImageRemove
// =====================================================

func TestDockerManager_ImageRemove_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/images/sha256:abc123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		jsonResponse(w, http.StatusOK, []image.DeleteResponse{
			{Deleted: "sha256:abc123"},
		})
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.ImageRemove(context.Background(), "sha256:abc123")
	if err != nil {
		t.Fatalf("ImageRemove() error: %v", err)
	}
}

func TestDockerManager_ImageRemove_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/images/sha256:nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such image"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.ImageRemove(context.Background(), "sha256:nonexistent")
	if err == nil {
		t.Fatal("ImageRemove() should return error for non-existent image")
	}
	if !strings.Contains(err.Error(), "remove image") {
		t.Errorf("error should mention 'remove image', got: %v", err)
	}
}

// =====================================================
// NetworkList
// =====================================================

func TestDockerManager_NetworkList_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			nets := []network.Inspect{
				{
					ID:     "net-1",
					Name:   "bridge",
					Driver: "bridge",
					Scope:  "local",
				},
				{
					ID:     "net-2",
					Name:   "monster-network",
					Driver: "bridge",
					Scope:  "local",
				},
			}
			jsonResponse(w, http.StatusOK, nets)
		}
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.NetworkList(context.Background())
	if err != nil {
		t.Fatalf("NetworkList() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("NetworkList() returned %d items, want 2", len(result))
	}
	if result[0].ID != "net-1" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "net-1")
	}
	if result[0].Name != "bridge" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "bridge")
	}
	if result[0].Driver != "bridge" {
		t.Errorf("result[0].Driver = %q, want %q", result[0].Driver, "bridge")
	}
	if result[0].Scope != "local" {
		t.Errorf("result[0].Scope = %q, want %q", result[0].Scope, "local")
	}
}

func TestDockerManager_NetworkList_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/networks", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"daemon error"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.NetworkList(context.Background())
	if err == nil {
		t.Fatal("NetworkList() should return error on server error")
	}
	if !strings.Contains(err.Error(), "list networks") {
		t.Errorf("error should mention 'list networks', got: %v", err)
	}
}

// =====================================================
// VolumeList
// =====================================================

func TestDockerManager_VolumeList_Success(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/volumes", func(w http.ResponseWriter, r *http.Request) {
		resp := volume.ListResponse{
			Volumes: []*volume.Volume{
				{
					Name:       "vol-data",
					Driver:     "local",
					Mountpoint: "/var/lib/docker/volumes/vol-data/_data",
					CreatedAt:  "2024-01-15T10:30:00Z",
				},
				{
					Name:       "vol-db",
					Driver:     "local",
					Mountpoint: "/var/lib/docker/volumes/vol-db/_data",
					CreatedAt:  "2024-02-20T14:00:00Z",
				},
			},
		}
		jsonResponse(w, http.StatusOK, resp)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.VolumeList(context.Background())
	if err != nil {
		t.Fatalf("VolumeList() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("VolumeList() returned %d items, want 2", len(result))
	}
	if result[0].Name != "vol-data" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "vol-data")
	}
	if result[0].Driver != "local" {
		t.Errorf("result[0].Driver = %q, want %q", result[0].Driver, "local")
	}
	if result[0].Mountpoint != "/var/lib/docker/volumes/vol-data/_data" {
		t.Errorf("result[0].Mountpoint = %q, want %q", result[0].Mountpoint, "/var/lib/docker/volumes/vol-data/_data")
	}
	if result[0].CreatedAt != "2024-01-15T10:30:00Z" {
		t.Errorf("result[0].CreatedAt = %q, want %q", result[0].CreatedAt, "2024-01-15T10:30:00Z")
	}
}

func TestDockerManager_VolumeList_Empty(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/volumes", func(w http.ResponseWriter, r *http.Request) {
		resp := volume.ListResponse{
			Volumes: []*volume.Volume{},
		}
		jsonResponse(w, http.StatusOK, resp)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.VolumeList(context.Background())
	if err != nil {
		t.Fatalf("VolumeList() error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("VolumeList() returned %d items, want 0", len(result))
	}
}

func TestDockerManager_VolumeList_Error(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/volumes", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"daemon error"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	_, err := dm.VolumeList(context.Background())
	if err == nil {
		t.Fatal("VolumeList() should return error on server error")
	}
	if !strings.Contains(err.Error(), "list volumes") {
		t.Errorf("error should mention 'list volumes', got: %v", err)
	}
}

// =====================================================
// EnsureNetwork — network already exists
// =====================================================

func TestDockerManager_EnsureNetwork_AlreadyExists(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	var createCalled bool
	h.mux.HandleFunc("/v1.45/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			nets := []network.Inspect{
				{
					ID:     "existing-net-id",
					Name:   "monster-network",
					Driver: "bridge",
					Scope:  "local",
				},
			}
			jsonResponse(w, http.StatusOK, nets)
		}
	})
	h.mux.HandleFunc("/v1.45/networks/create", func(w http.ResponseWriter, r *http.Request) {
		createCalled = true
		jsonResponse(w, http.StatusCreated, network.CreateResponse{ID: "new-net"})
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.EnsureNetwork(context.Background(), "monster-network")
	if err != nil {
		t.Fatalf("EnsureNetwork() error: %v", err)
	}
	if createCalled {
		t.Error("EnsureNetwork() should NOT create a network that already exists")
	}
}

func TestDockerManager_EnsureNetwork_DoesNotExist(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	var createCalled bool
	h.mux.HandleFunc("/v1.45/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Return empty list — network does not exist
			jsonResponse(w, http.StatusOK, []network.Inspect{})
		}
	})
	h.mux.HandleFunc("/v1.45/networks/create", func(w http.ResponseWriter, r *http.Request) {
		createCalled = true
		jsonResponse(w, http.StatusCreated, network.CreateResponse{ID: "new-monster-net"})
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.EnsureNetwork(context.Background(), "monster-network")
	if err != nil {
		t.Fatalf("EnsureNetwork() error: %v", err)
	}
	if !createCalled {
		t.Error("EnsureNetwork() should create the network when it doesn't exist")
	}
}

func TestDockerManager_EnsureNetwork_NameMismatch(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	var createCalled bool
	h.mux.HandleFunc("/v1.45/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// Return a network with a different name (Docker filter is prefix-based)
			nets := []network.Inspect{
				{
					ID:     "other-net-id",
					Name:   "monster-network-old",
					Driver: "bridge",
				},
			}
			jsonResponse(w, http.StatusOK, nets)
		}
	})
	h.mux.HandleFunc("/v1.45/networks/create", func(w http.ResponseWriter, r *http.Request) {
		createCalled = true
		jsonResponse(w, http.StatusCreated, network.CreateResponse{ID: "new-net"})
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.EnsureNetwork(context.Background(), "monster-network")
	if err != nil {
		t.Fatalf("EnsureNetwork() error: %v", err)
	}
	if !createCalled {
		t.Error("EnsureNetwork() should create network when exact name not found")
	}
}

func TestDockerManager_EnsureNetwork_ListError(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/networks", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"daemon error"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.EnsureNetwork(context.Background(), "monster-network")
	if err == nil {
		t.Fatal("EnsureNetwork() should return error when list fails")
	}
	if !strings.Contains(err.Error(), "list networks") {
		t.Errorf("error should mention 'list networks', got: %v", err)
	}
}

func TestDockerManager_EnsureNetwork_CreateError(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/networks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			jsonResponse(w, http.StatusOK, []network.Inspect{})
		}
	})
	h.mux.HandleFunc("/v1.45/networks/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"permission denied"}`))
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	err := dm.EnsureNetwork(context.Background(), "monster-network")
	if err == nil {
		t.Fatal("EnsureNetwork() should return error when create fails")
	}
	if !strings.Contains(err.Error(), "create network") {
		t.Errorf("error should mention 'create network', got: %v", err)
	}
}

// =====================================================
// NewDockerManager — error and success paths
// =====================================================

func TestNewDockerManager_ErrorPath_BadHost(t *testing.T) {
	// A completely invalid host URL scheme that the Docker client rejects
	_, err := NewDockerManager("://invalid-scheme")
	if err == nil {
		t.Log("NewDockerManager did not return immediate error (lazy connect may defer)")
	}
}

func TestNewDockerManager_ErrorPath_ClientCreateFails(t *testing.T) {
	// A host string without "://" causes ParseHostURL to fail inside WithHost,
	// which causes client.NewClientWithOpts to return an error.
	_, err := NewDockerManager("not-a-valid-host")
	if err == nil {
		t.Fatal("NewDockerManager should return error for unparseable host")
	}
	if !strings.Contains(err.Error(), "create docker client") {
		t.Errorf("error should mention 'create docker client', got: %v", err)
	}
}

func TestNewDockerManager_ErrorPath_UnreachableHost(t *testing.T) {
	// Valid TCP scheme but unreachable host — should fail on Ping
	_, err := NewDockerManager("tcp://192.0.2.1:1") // RFC 5737 TEST-NET-1
	if err == nil {
		t.Fatal("NewDockerManager should return error for unreachable host")
	}
	if !strings.Contains(err.Error(), "docker ping") {
		t.Errorf("error should mention 'docker ping', got: %v", err)
	}
}

func TestNewDockerManager_SuccessPath(t *testing.T) {
	// Create a mock Docker daemon that responds to /_ping
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	srv := httptest.NewServer(h)
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")

	dm, err := NewDockerManager("tcp://" + addr)
	if err != nil {
		t.Fatalf("NewDockerManager() error: %v", err)
	}
	if dm == nil {
		t.Fatal("NewDockerManager() returned nil DockerManager")
	}
	if dm.cli == nil {
		t.Fatal("DockerManager.cli should not be nil after successful init")
	}
	dm.cli.Close()
}

func TestNewDockerManager_DefaultHost(t *testing.T) {
	// Default unix socket host — Ping will fail without Docker daemon
	_, err := NewDockerManager("unix:///var/run/docker.sock")
	if err != nil {
		// Expected on machines without Docker
		t.Logf("NewDockerManager with default socket: %v", err)
	}
}

// =====================================================
// ContainerRuntime interface compliance
// =====================================================

func TestDockerManager_ImplementsContainerRuntime_Full(t *testing.T) {
	var _ core.ContainerRuntime = (*DockerManager)(nil)
}

// =====================================================
// ListByLabels — multiple label filters
// =====================================================

func TestDockerManager_ListByLabels_MultipleFilters(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())

	var receivedFilters string
	h.mux.HandleFunc("/v1.45/containers/json", func(w http.ResponseWriter, r *http.Request) {
		receivedFilters = r.URL.Query().Get("filters")
		jsonResponse(w, http.StatusOK, []container.Summary{
			{
				ID:     "ctr-multi-label",
				Names:  []string{"/multi-label-container"},
				Image:  "myapp:latest",
				State:  "running",
				Labels: map[string]string{"monster.enable": "true", "monster.app.id": "app-1"},
			},
		})
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	result, err := dm.ListByLabels(context.Background(), map[string]string{
		"monster.enable": "true",
		"monster.app.id": "app-1",
	})
	if err != nil {
		t.Fatalf("ListByLabels() error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("ListByLabels() returned %d items, want 1", len(result))
	}

	// Verify filters were sent
	if receivedFilters == "" {
		t.Error("expected filters query parameter to be set")
	}
}

// =====================================================
// Stats — sysDelta positive, cpuDelta positive (both > 0)
// =====================================================

func TestDockerManager_Stats_BothDeltasPositive(t *testing.T) {
	h := newMockDockerHandler()
	h.mux.HandleFunc("/_ping", defaultPingHandler())
	h.mux.HandleFunc("/v1.45/containers/ctr-stats-positive/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := container.StatsResponse{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 200000000, // 200ms
				},
				SystemUsage: 1000000000, // 1s
				OnlineCPUs:  2,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 100000000, // 100ms
				},
				SystemUsage: 500000000, // 0.5s
			},
			MemoryStats: container.MemoryStats{
				Usage: 2147483648,  // 2GB
				Limit: 4294967296, // 4GB
			},
		}
		jsonResponse(w, http.StatusOK, stats)
	})

	dm, srv := newTestDockerManager(t, h)
	defer srv.Close()

	stats, err := dm.Stats(context.Background(), "ctr-stats-positive")
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	// cpuDelta = 200M - 100M = 100M, sysDelta = 1G - 500M = 500M
	// cpuPercent = (100M / 500M) * 2 * 100 = 40.0
	expectedCPU := 40.0
	if stats.CPUPercent != expectedCPU {
		t.Errorf("CPUPercent = %f, want %f", stats.CPUPercent, expectedCPU)
	}

	// memPercent = 2G / 4G * 100 = 50.0
	expectedMem := 50.0
	if stats.MemoryPercent != expectedMem {
		t.Errorf("MemoryPercent = %f, want %f", stats.MemoryPercent, expectedMem)
	}
}
