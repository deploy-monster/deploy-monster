package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LogDownloadHandler exports container logs as a downloadable file.
type LogDownloadHandler struct {
	runtime core.ContainerRuntime
}

func NewLogDownloadHandler(runtime core.ContainerRuntime) *LogDownloadHandler {
	return &LogDownloadHandler{runtime: runtime}
}

// Download handles GET /api/v1/apps/{id}/logs/download
func (h *LogDownloadHandler) Download(w http.ResponseWriter, r *http.Request) {
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no container found")
		return
	}

	reader, err := h.runtime.Logs(r.Context(), containers[0].ID, "5000", false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read logs")
		return
	}
	defer reader.Close()

	filename := fmt.Sprintf("%s-logs-%s.txt", appID[:8], time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+safeFilename(filename))

	ctx := r.Context()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := reader.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
