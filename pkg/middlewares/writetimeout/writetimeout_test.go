package writetimeout

import (
	"cmp"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ptypes "github.com/traefik/paerser/types"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/middlewares"
	"github.com/traefik/traefik/v3/pkg/middlewares/buffering"
	"github.com/traefik/traefik/v3/pkg/middlewares/capture"
	"github.com/traefik/traefik/v3/pkg/middlewares/circuitbreaker"
	"github.com/traefik/traefik/v3/pkg/middlewares/compress"
	"github.com/traefik/traefik/v3/pkg/middlewares/grpcweb"
	"github.com/traefik/traefik/v3/pkg/middlewares/headers"
	"github.com/traefik/traefik/v3/pkg/middlewares/recovery"
	"github.com/traefik/traefik/v3/pkg/middlewares/retry"
)

// deadlineRecorder records the last write deadline set through http.ResponseController.
type deadlineRecorder struct {
	http.ResponseWriter

	deadline    time.Time
	deadlineSet bool
}

func (d *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	d.deadline = deadline
	d.deadlineSet = true
	return nil
}

func TestWriteTimeout_SetsWriteDeadline(t *testing.T) {
	testCases := []struct {
		desc       string
		timeout    ptypes.Duration
		expectZero bool
	}{
		{
			desc:       "positive timeout sets a deadline",
			timeout:    ptypes.Duration(time.Hour),
			expectZero: false,
		},
		{
			desc:       "zero timeout clears the deadline",
			timeout:    0,
			expectZero: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var nextCalled bool
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				nextCalled = true
			})

			handler, err := New(t.Context(), next, dynamic.WriteTimeout{Timeout: test.timeout}, "foo")
			require.NoError(t, err)

			recorder := &deadlineRecorder{ResponseWriter: httptest.NewRecorder()}
			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)

			start := time.Now()
			handler.ServeHTTP(recorder, req)

			assert.True(t, nextCalled)
			require.True(t, recorder.deadlineSet)

			if test.expectZero {
				assert.True(t, recorder.deadline.IsZero())
				return
			}

			assert.WithinDuration(t, start.Add(time.Duration(test.timeout)), recorder.deadline, time.Minute)
		})
	}
}

func TestWriteTimeout_NegativeTimeout(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

	_, err := New(t.Context(), next, dynamic.WriteTimeout{Timeout: ptypes.Duration(-time.Second)}, "foo")
	assert.Error(t, err)
}

func TestWriteTimeout_UnsupportedResponseWriter(t *testing.T) {
	var nextCalled bool
	next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		rw.WriteHeader(http.StatusOK)
	})

	handler, err := New(t.Context(), next, dynamic.WriteTimeout{Timeout: ptypes.Duration(time.Second)}, "foo")
	require.NoError(t, err)

	// httptest.ResponseRecorder implements neither SetWriteDeadline nor Unwrap,
	// so the deadline cannot be set: the middleware must log and serve the request anyway.
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)

	handler.ServeHTTP(recorder, req)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, recorder.Code)
}

// TestWriteTimeout_ThroughResponseWriterWrappers guards the Unwrap chain, which is how
// http.ResponseController reaches SetWriteDeadline when the request did not go through WrapEntryPointHandler.
func TestWriteTimeout_ThroughResponseWriterWrappers(t *testing.T) {
	testCases := []struct {
		desc    string
		headers http.Header
		wrap    func(t *testing.T, next http.Handler) http.Handler
	}{
		{
			desc: "capture",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := capture.Wrap(next)
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc: "recovery",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := recovery.New(t.Context(), next)
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc: "retry",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := retry.New(t.Context(), next, dynamic.Retry{Attempts: 2}, retry.Listeners{}, "retry")
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc: "headers (ResponseModifier)",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := headers.NewHeader(next, dynamic.Headers{
					CustomResponseHeaders: map[string]string{"X-Test": "test"},
				})
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc:    "compress (gzip)",
			headers: http.Header{"Accept-Encoding": []string{"gzip"}},
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := compress.New(t.Context(), next, dynamic.Compress{Encodings: []string{"gzip"}}, "compress")
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc:    "compress (brotli)",
			headers: http.Header{"Accept-Encoding": []string{"br"}},
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := compress.New(t.Context(), next, dynamic.Compress{Encodings: []string{"br"}}, "compress")
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc:    "compress (zstd)",
			headers: http.Header{"Accept-Encoding": []string{"zstd"}},
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := compress.New(t.Context(), next, dynamic.Compress{Encodings: []string{"zstd"}}, "compress")
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc: "ResponseModifier",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
					next.ServeHTTP(middlewares.NewResponseModifier(rw, req, nil), req)
				})
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(http.StatusOK)
			})

			writeTimeoutHandler, err := New(t.Context(), next, dynamic.WriteTimeout{Timeout: ptypes.Duration(time.Hour)}, "foo")
			require.NoError(t, err)

			recorder := &deadlineRecorder{ResponseWriter: httptest.NewRecorder()}
			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			maps.Copy(req.Header, test.headers)

			start := time.Now()
			test.wrap(t, writeTimeoutHandler).ServeHTTP(recorder, req)

			require.True(t, recorder.deadlineSet)
			assert.WithinDuration(t, start.Add(time.Hour), recorder.deadline, time.Minute)
		})
	}
}

// opaqueResponseWriter stands for the wrappers that do not implement Unwrap, such as those of plugins.
type opaqueResponseWriter struct {
	http.ResponseWriter
}

func TestWriteTimeout_ThroughEntryPointHandler(t *testing.T) {
	testCases := []struct {
		desc    string
		method  string
		headers http.Header
		wrap    func(t *testing.T, next http.Handler) http.Handler
	}{
		{
			desc: "opaque wrapper",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
					next.ServeHTTP(&opaqueResponseWriter{ResponseWriter: rw}, req)
				})
			},
		},
		{
			desc: "buffering",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := buffering.New(t.Context(), next, dynamic.Buffering{}, "buffering")
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc: "circuitBreaker",
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				handler, err := circuitbreaker.New(t.Context(), next, dynamic.CircuitBreaker{Expression: "NetworkErrorRatio() > 0.5"}, "circuitbreaker")
				require.NoError(t, err)
				return handler
			},
		},
		{
			desc:    "grpcWeb",
			method:  http.MethodPost,
			headers: http.Header{"Content-Type": []string{"application/grpc-web"}},
			wrap: func(t *testing.T, next http.Handler) http.Handler {
				t.Helper()
				return grpcweb.New(t.Context(), next, dynamic.GrpcWeb{}, "grpcweb")
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(http.StatusOK)
			})

			writeTimeoutHandler, err := New(t.Context(), next, dynamic.WriteTimeout{Timeout: ptypes.Duration(time.Hour)}, "foo")
			require.NoError(t, err)

			entryPointHandler, err := WrapEntryPointHandler(test.wrap(t, writeTimeoutHandler))
			require.NoError(t, err)

			recorder := &deadlineRecorder{ResponseWriter: httptest.NewRecorder()}
			req := httptest.NewRequest(cmp.Or(test.method, http.MethodGet), "http://localhost", nil)
			maps.Copy(req.Header, test.headers)

			start := time.Now()
			entryPointHandler.ServeHTTP(recorder, req)

			require.True(t, recorder.deadlineSet)
			assert.WithinDuration(t, start.Add(time.Hour), recorder.deadline, time.Minute)
		})
	}
}
