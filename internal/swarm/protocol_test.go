package swarm

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestAgentProtocolDecoder_AcceptsFramedMessage(t *testing.T) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(core.AgentMessage{
		ID:   "msg-1",
		Type: core.AgentMsgPing,
	}); err != nil {
		t.Fatalf("encode: %v", err)
	}

	var msg core.AgentMessage
	if err := newAgentProtocolDecoder(&buf).Decode(&msg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.ID != "msg-1" || msg.Type != core.AgentMsgPing {
		t.Fatalf("message = %#v", msg)
	}
}

func TestAgentProtocolDecoder_RejectsTrailingJSONInFrame(t *testing.T) {
	input := strings.NewReader(`{"id":"one","type":"ping"} {"id":"two","type":"ping"}` + "\n")

	var msg core.AgentMessage
	err := newAgentProtocolDecoder(input).Decode(&msg)
	if err == nil {
		t.Fatal("expected trailing JSON error")
	}
	if !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("error = %q", err)
	}
}

func TestAgentProtocolDecoder_RejectsOversizedFrame(t *testing.T) {
	input := strings.NewReader(`{"id":"` + strings.Repeat("a", maxAgentMessageBytes) + `","type":"ping"}` + "\n")

	var msg core.AgentMessage
	err := newAgentProtocolDecoder(input).Decode(&msg)
	if err == nil {
		t.Fatal("expected oversized frame error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %q", err)
	}
}

func TestAgentClient_ReadLoop_RejectsOversizedMessage(t *testing.T) {
	clientConn, masterConn := net.Pipe()
	defer clientConn.Close()
	defer masterConn.Close()

	client := &AgentClient{
		conn:    clientConn,
		decoder: newAgentProtocolDecoder(clientConn),
		logger:  discardLogger(),
		sem:     make(chan struct{}, maxConcurrentHandlers),
	}

	go func() {
		_, _ = masterConn.Write([]byte(`{"id":"` + strings.Repeat("a", maxAgentMessageBytes) + `","type":"ping"}` + "\n"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := client.readLoop(ctx)
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %q", err)
	}
}

func TestAgentServer_ReadLoop_RejectsOversizedMessage(t *testing.T) {
	serverConn, agentConn := net.Pipe()
	defer serverConn.Close()
	defer agentConn.Close()

	s := NewAgentServer(nil, "token", discardLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "oversized-agent",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  newAgentProtocolDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
		lastSeen: time.Now(),
	}

	s.mu.Lock()
	s.agents[ac.ServerID] = ac
	s.wg.Add(1)
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.readLoop(ac)
		close(done)
	}()

	_, _ = agentConn.Write([]byte(`{"id":"` + strings.Repeat("a", maxAgentMessageBytes) + `","type":"pong"}` + "\n"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not exit after oversized frame")
	}

	if got := s.ConnectedAgents(); len(got) != 0 {
		t.Fatalf("connected agents = %#v", got)
	}
}
