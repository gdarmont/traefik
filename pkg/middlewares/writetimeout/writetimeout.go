package writetimeout

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/middlewares"
)

const typeName = "WriteTimeout"

// writeTimeout overrides, for the matching requests, the response write deadline
// otherwise enforced by the EntryPoint writeTimeout.
type writeTimeout struct {
	next    http.Handler
	timeout time.Duration
	name    string

	// The wrapper chain is fixed for a given route, so an unsupported ResponseWriter
	// makes the middleware a no-op for every request: warn once instead of per request.
	warnUnsupported sync.Once
}

// New builds a new WriteTimeout middleware.
func New(ctx context.Context, next http.Handler, config dynamic.WriteTimeout, name string) (http.Handler, error) {
	middlewares.GetLogger(ctx, name, typeName).Debug().Msg("Creating middleware")

	if config.Timeout < 0 {
		return nil, fmt.Errorf("timeout must not be negative: %s", time.Duration(config.Timeout))
	}

	return &writeTimeout{
		next:    next,
		timeout: time.Duration(config.Timeout),
		name:    name,
	}, nil
}

func (w *writeTimeout) GetTracingInformation() (string, string) {
	return w.name, typeName
}

func (w *writeTimeout) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// A zero deadline disables the write timeout, so that long-lived streaming responses
	// (e.g. Server-Sent Events) can outlive the EntryPoint writeTimeout.
	var deadline time.Time
	if w.timeout > 0 {
		deadline = time.Now().Add(w.timeout)
	}

	if err := http.NewResponseController(rw).SetWriteDeadline(deadline); err != nil {
		// net/http returns an error wrapping ErrNotSupported when no ResponseWriter in the
		// chain exposes SetWriteDeadline, which makes this middleware a silent no-op.
		if errors.Is(err, http.ErrNotSupported) {
			w.warnUnsupported.Do(func() {
				middlewares.GetLogger(req.Context(), w.name, typeName).
					Warn().Msg("Write deadline is not supported by the ResponseWriter chain, the middleware has no effect")
			})
		} else {
			middlewares.GetLogger(req.Context(), w.name, typeName).
				Debug().Err(err).Msg("Unable to set write deadline")
		}
	}

	w.next.ServeHTTP(rw, req)
}
