package api

import (
	"net/http/httptest"
	"testing"
)

func BenchmarkRespondOK(b *testing.B) {
	data := map[string]string{"key": "value", "name": "test"}

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		RespondOK(rr, data)
	}
}

func BenchmarkRespondError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		RespondError(rr, 400, "bad_request", "invalid input")
	}
}

func BenchmarkRespondPaginated(b *testing.B) {
	data := []string{"a", "b", "c", "d", "e"}

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		RespondPaginated(rr, data, 1, 20, 100)
	}
}
