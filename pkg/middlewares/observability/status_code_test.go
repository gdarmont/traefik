package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The WriteTimeout middleware relies on Unwrap to reach SetWriteDeadline
// through the wrapper chain.
func TestStatusCodeRecorder_Unwrap(t *testing.T) {
	rw := httptest.NewRecorder()

	recorder := newStatusCodeRecorder(rw, http.StatusOK)

	assert.Same(t, rw, recorder.Unwrap())
}
