// Package proxy provides proxy and proxy middleware interfaces and implementations.
package proxy

import (
	"context"
	"errors"
	"io"

	"github.com/devopsfaith/krakend/config"
)

// Namespace to be used in extra config
const Namespace = "github.com/devopsfaith/krakend/proxy"

// Metadata is the Metadata of the Response which contains Headers and StatusCode
type Metadata struct {
	Headers    map[string][]string
	StatusCode int
	IsRequired bool
}

// Response is the entity returned by the proxy
type Response struct {
	Data       map[string]interface{}
	IsComplete bool
	Metadata   Metadata
	Io         io.Reader
}

// readCloserWrapper is Io.Reader which is closed when the Context is closed or canceled
type readCloserWrapper struct {
	ctx context.Context
	rc  io.ReadCloser
}

// NewReadCloserWrapper Creates a new closeable io.Read
func NewReadCloserWrapper(ctx context.Context, in io.ReadCloser) io.Reader {
	wrapper := readCloserWrapper{ctx, in}
	go wrapper.closeOnCancel()
	return wrapper
}

func (w readCloserWrapper) Read(b []byte) (int, error) {
	return w.rc.Read(b)
}

// closeOnCancel closes the io.Reader when the context is Done
func (w readCloserWrapper) closeOnCancel() {
	<-w.ctx.Done()
	w.rc.Close()
}

var (
	// ErrNoBackends is the error returned when an endpoint has no backends defined
	ErrNoBackends = errors.New("all endpoints must have at least one backend")
	// ErrTooManyBackends is the error returned when an endpoint has too many backends defined
	ErrTooManyBackends = errors.New("too many backends for this proxy")
	// ErrTooManyProxies is the error returned when a middleware has too many proxies defined
	ErrTooManyProxies = errors.New("too many proxies for this proxy middleware")
	// ErrNotEnoughProxies is the error returned when an endpoint has not enough proxies defined
	ErrNotEnoughProxies = errors.New("not enough proxies for this endpoint")
)

// Proxy processes a request in a given context and returns a response and an error
type Proxy func(ctx context.Context, request *Request) (*Response, error)

// BackendFactory creates a proxy based on the received backend configuration
type BackendFactory func(remote *config.Backend) Proxy

// Middleware adds a middleware, decorator or wrapper over a collection of proxies,
// exposing a proxy interface.
//
// Proxy middlewares can be stacked:
//	var p Proxy
//	p := EmptyMiddleware(NoopProxy)
//	response, err := p(ctx, r)
type Middleware func(next ...Proxy) Proxy

// EmptyMiddleware is a dummy middleware, useful for testing and fallback
func EmptyMiddleware(next ...Proxy) Proxy {
	if len(next) > 1 {
		panic(ErrTooManyProxies)
	}
	return next[0]
}

// NoopProxy is a do nothing proxy, useful for testing
func NoopProxy(_ context.Context, _ *Request) (*Response, error) { return nil, nil }
