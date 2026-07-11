package swarm

import (
	"context"
	"testing"
	"time"
)

// =============================================================================
// Client — NewAgentClient with mTLS (client.go:63)
// =============================================================================

func TestNewAgentClient_WithCertFiles(t *testing.T) {
	// When certFile is non-empty but doesn't exist, the client still
	// gets a tlsConfig but will fail when trying to load the certs
	client := NewAgentClient("https://master:8080", "server1", "token", "1.0", nil, discardLogger(), "/nonexistent/cert.pem", "/nonexistent/key.pem", "")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.tlsConfig == nil {
		t.Error("expected TLS config when cert and key are provided")
	}
}

func TestNewAgentClient_WithoutTLS(t *testing.T) {
	client := NewAgentClient("https://master:8080", "server1", "token", "1.0", nil, discardLogger(), "", "", "")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.tlsConfig != nil {
		t.Error("expected no TLS config when no cert/key provided")
	}
}

func TestNewAgentClient_DefaultPort(t *testing.T) {
	client := NewAgentClient("https://master:8080", "server1", "token", "1.0", nil, discardLogger(), "", "", "")
	client.SetDefaultPort(0) // should be ignored
	client.SetDefaultPort(9999)
	if client.defaultPort != 9999 {
		t.Errorf("expected defaultPort 9999, got %d", client.defaultPort)
	}
}

// =============================================================================
// Client — requireRuntime (client.go:115)
// =============================================================================

func TestRequireRuntime_Nil(t *testing.T) {
	client := NewAgentClient("https://master:8080", "server1", "token", "1.0", nil, discardLogger(), "", "", "")
	_, err := client.requireRuntime()
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
}

// =============================================================================
// RemoteExecutor — Logs follow error (remote.go:61)
// =============================================================================

func TestRemoteExecutor_LogsFollowError(t *testing.T) {
	remote := &RemoteExecutor{}
	_, err := remote.Logs(context.Background(), "container1", "100", true)
	if err == nil || err.Error() != "follow mode not supported over agent protocol" {
		t.Fatalf("expected follow-mode error, got: %v", err)
	}
}

// =============================================================================
// Server — heartbeat tick with no agents
// =============================================================================

func TestServer_HeartbeatTickNoAgents(t *testing.T) {
	server := NewAgentServer(nil, "token", discardLogger())
	server.heartbeatTick(time.Second)
	// Should not panic
}

// =============================================================================
// Module — basic identity (module.go)
// =============================================================================

func TestSwarmModule_ID(t *testing.T) {
	m := &Module{}
	if m.ID() == "" {
		t.Error("expected non-empty ID")
	}
}

func TestSwarmModule_Name(t *testing.T) {
	m := &Module{}
	if m.Name() == "" {
		t.Error("expected non-empty Name")
	}
}

func TestSwarmModule_Version(t *testing.T) {
	m := &Module{}
	if m.Version() == "" {
		t.Error("expected non-empty Version")
	}
}

// =============================================================================
// AgentClient — SetBuildModule
// =============================================================================

func TestAgentClient_SetBuildModuleExtra(t *testing.T) {
	client := NewAgentClient("https://master:8080", "server1", "token", "1.0", nil, discardLogger(), "", "", "")
	client.SetBuildModule(nil) // shouldn't panic
	if client.buildMod != nil {
		t.Error("expected nil buildMod")
	}
}

// =============================================================================
// Protocol — newAgentProtocolDecoder (protocol.go)
// =============================================================================

func TestProtocolDecoder_New(t *testing.T) {
	d := newAgentProtocolDecoder(nil) // nil reader — will fail on decode
	if d == nil {
		t.Fatal("expected non-nil decoder")
	}
}
