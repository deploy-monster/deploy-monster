package handlers

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RestartHistoryHandler tracks container restart events.
type RestartHistoryHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	bolt    core.BoltStorer
}

func NewRestartHistoryHandler(store core.Store, runtime core.ContainerRuntime) *RestartHistoryHandler {
	return &RestartHistoryHandler{store: store, runtime: runtime}
}

// SetBolt wires the persistence backend used to store events.
// Called by the router after construction so the bolt dependency is
// opt-in (some unit tests construct the handler without a backing
// store).
func (h *RestartHistoryHandler) SetBolt(b core.BoltStorer) { h.bolt = b }

// RestartEvent records when and why a container restarted. The persisted
// shape is intentionally compact so a high-churn app doesn't bloat the
// BBolt file. Per-event TTL is applied at write time so old entries roll
// off without a separate sweeper.
type RestartEvent struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	ContainerID string    `json:"container_id,omitempty"`
	Reason      string    `json:"reason"` // manual, crash, deploy, autorestart
	Source      string    `json:"source"` // event type that produced the entry
	Timestamp   time.Time `json:"timestamp"`
}

// RestartHistoryBucket is the BBolt bucket used by the event subscriber
// in api/router.go to persist restart records. Reads route through the
// bolt auto-create path so the bucket appears on first write.
const RestartHistoryBucket = "restart_history"

// RestartHistoryRetentionSeconds caps how long a single record lives in
// BBolt. 30 days strikes a balance between operator usefulness ("did we
// crash this week?") and disk pressure on a chatty app.
const RestartHistoryRetentionSeconds = 30 * 24 * 3600

// SubscribeRestartHistory wires bolt persistence to the relevant lifecycle
// events. Returns the subscription IDs so the caller can Unsubscribe on
// shutdown if desired (the router pattern doesn't, since the bus and
// bolt outlive the request loop). Lives in this file so the contract
// between the persisted shape and the read path stays in one place.
func SubscribeRestartHistory(events *core.EventBus, bolt core.BoltStorer) {
	if events == nil || bolt == nil {
		return
	}

	persist := func(reason, source string, appID, containerID string) {
		ev := RestartEvent{
			ID:          core.GenerateID(),
			AppID:       appID,
			ContainerID: containerID,
			Reason:      reason,
			Source:      source,
			Timestamp:   time.Now().UTC(),
		}
		_ = bolt.Set(RestartHistoryBucket, appID+":"+ev.ID, ev, RestartHistoryRetentionSeconds)
	}

	events.SubscribeAsync(core.EventContainerDied, func(_ context.Context, e core.Event) error {
		if d, ok := e.Data.(core.DeployEventData); ok {
			persist("crash", e.Type, d.AppID, d.ContainerID)
		}
		return nil
	})
	events.SubscribeAsync(core.EventAppStarted, func(_ context.Context, e core.Event) error {
		appID, action, container := extractAppAction(e)
		if appID == "" {
			return nil
		}
		reason := "manual"
		if action == "restart" {
			reason = "restart"
		} else if e.Source == "autorestart" {
			reason = "autorestart"
		}
		persist(reason, e.Type, appID, container)
		return nil
	})
	events.SubscribeAsync(core.EventAppDeployed, func(_ context.Context, e core.Event) error {
		switch d := e.Data.(type) {
		case core.AppEventData:
			persist("deploy", e.Type, d.AppID, "")
		case core.DeployEventData:
			persist("deploy", e.Type, d.AppID, d.ContainerID)
		}
		return nil
	})
}

// extractAppAction pulls app_id / action / container_id out of the
// generic map[string]string payload that AppHandler.Start/Restart/Stop
// publish, since AppEventData isn't always used there.
func extractAppAction(e core.Event) (appID, action, containerID string) {
	switch d := e.Data.(type) {
	case core.AppEventData:
		return d.AppID, "", ""
	case map[string]string:
		return d["id"], d["action"], d["container_id"]
	}
	return "", "", ""
}

// List handles GET /api/v1/apps/{id}/restarts.
// Reads persisted events filtered to the calling app, newest first.
func (h *RestartHistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	resp := map[string]any{
		"app_id": appID,
		"data":   []RestartEvent{},
		"total":  0,
	}
	if h.runtime != nil {
		if containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
			"monster.app.id": appID,
		}); err == nil && len(containers) > 0 {
			resp["container_id"] = containers[0].ID[:12]
		}
	}

	if h.bolt == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	keys, err := h.bolt.List(RestartHistoryBucket)
	if err != nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	prefix := appID + ":"
	events := make([]RestartEvent, 0)
	for _, k := range keys {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		var ev RestartEvent
		if h.bolt.Get(RestartHistoryBucket, k, &ev) == nil {
			events = append(events, ev)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Timestamp.After(events[j].Timestamp) })

	const cap = 200
	if len(events) > cap {
		events = events[:cap]
	}
	resp["data"] = events
	resp["total"] = len(events)
	writeJSON(w, http.StatusOK, resp)
}
