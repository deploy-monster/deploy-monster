package ws

import (
	"strings"
	"testing"
)

func TestWriteSSEEventEscapesFieldBreakout(t *testing.T) {
	var b strings.Builder

	writeSSEEvent(&b, "error\nid: injected", "first line\nevent: injected\r\nretry: 1")

	got := b.String()
	if strings.Contains(got, "\nid: injected") {
		t.Fatalf("event name escaped into id field: %q", got)
	}
	if strings.Contains(got, "\nevent: injected") {
		t.Fatalf("data escaped into event field: %q", got)
	}
	if !strings.Contains(got, "event: error id: injected\n") {
		t.Fatalf("event name was not sanitized as expected: %q", got)
	}
	if !strings.Contains(got, "data: event: injected\n") {
		t.Fatalf("multiline data was not emitted as data lines: %q", got)
	}
	if !strings.Contains(got, "data: retry: 1\n") {
		t.Fatalf("CRLF data line was not normalized: %q", got)
	}
}

func TestWriteSSECommentEscapesFieldBreakout(t *testing.T) {
	var b strings.Builder

	writeSSEComment(&b, "keepalive\nevent: injected")

	got := b.String()
	if strings.Contains(got, "\nevent: injected") {
		t.Fatalf("comment escaped into event field: %q", got)
	}
	if got != ": keepalive event: injected\n\n" {
		t.Fatalf("comment = %q", got)
	}
}
