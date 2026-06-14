package ws

import (
	"fmt"
	"io"
	"strings"
)

func writeSSEEvent(w io.Writer, event, data string) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", sanitizeSSEField(event))
	}
	writeSSEData(w, data)
	_, _ = io.WriteString(w, "\n")
}

func writeSSEData(w io.Writer, data string) {
	if data == "" {
		_, _ = io.WriteString(w, "data: \n")
		return
	}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSuffix(line, "\r")
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}
}

func writeSSEComment(w io.Writer, comment string) {
	_, _ = fmt.Fprintf(w, ": %s\n\n", sanitizeSSEField(comment))
}

func sanitizeSSEField(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
