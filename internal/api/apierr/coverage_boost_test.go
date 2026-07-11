package apierr

import (
	"errors"
	"net/http/httptest"
	"testing"
)

// errWriter wraps httptest.ResponseRecorder and returns an error on Write
// to exercise the json.Encode error branch in Write().
type errWriter struct {
	*httptest.ResponseRecorder
	failAfter int // number of successful writes before failing
	written   int
}

func (w *errWriter) Write(b []byte) (int, error) {
	if w.written >= w.failAfter {
		return 0, errors.New("write failed")
	}
	n, err := w.ResponseRecorder.Write(b)
	w.written += n
	return n, err
}

func TestWrite_JSONEncodeError(t *testing.T) {
	rr := httptest.NewRecorder()
	ew := &errWriter{ResponseRecorder: rr, failAfter: 0}
	// Write with a writer that immediately fails — should log error but not panic
	Write(ew, 500, "internal error")
	// No panic means success; the error branch was exercised
}
