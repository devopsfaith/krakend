// SPDX-License-Identifier: Apache-2.0
package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/luraproject/lura/config"
	"github.com/luraproject/lura/encoding"
	"github.com/luraproject/lura/transport/http/client"
)

var httpProxy = CustomHTTPProxyFactory(client.NewHTTPClient)

// HTTPProxyFactory returns a BackendFactory. The Proxies it creates will use the received net/http.Client
func HTTPProxyFactory(client *http.Client) BackendFactory {
	return CustomHTTPProxyFactory(func(_ context.Context) *http.Client { return client })
}

// CustomHTTPProxyFactory returns a BackendFactory. The Proxies it creates will use the received HTTPClientFactory
func CustomHTTPProxyFactory(cf client.HTTPClientFactory) BackendFactory {
	return func(backend *config.Backend) Proxy {
		return NewHTTPProxy(backend, cf, backend.Decoder)
	}
}

// NewHTTPProxy creates a http proxy with the injected configuration, HTTPClientFactory and Decoder
func NewHTTPProxy(remote *config.Backend, cf client.HTTPClientFactory, decode encoding.Decoder) Proxy {
	return NewHTTPProxyWithHTTPExecutor(remote, client.DefaultHTTPRequestExecutor(cf), decode)
}

// NewHTTPProxyWithHTTPExecutor creates a http proxy with the injected configuration, HTTPRequestExecutor and Decoder
func NewHTTPProxyWithHTTPExecutor(remote *config.Backend, re client.HTTPRequestExecutor, dec encoding.Decoder) Proxy {
	if remote.Encoding == encoding.NOOP {
		return NewHTTPProxyDetailed(remote, re, client.NoOpHTTPStatusHandler, NoOpHTTPResponseParser)
	}

	ef := NewEntityFormatter(remote)
	rp := DefaultHTTPResponseParserFactory(HTTPResponseParserConfig{dec, ef})
	return NewHTTPProxyDetailed(remote, re, client.GetHTTPStatusHandler(remote), rp)
}

// NewHTTPProxyDetailed creates a http proxy with the injected configuration, HTTPRequestExecutor,
// Decoder and HTTPResponseParser
func NewHTTPProxyDetailed(remote *config.Backend, re client.HTTPRequestExecutor, ch client.HTTPStatusHandler, rp HTTPResponseParser) Proxy {
	return func(ctx context.Context, request *Request) (*Response, error) {
		requestToBakend, err := http.NewRequest(strings.ToTitle(request.Method), request.URL.String(), request.Body)
		if err != nil {
			return nil, err
		}
		requestToBakend.Header = make(map[string][]string, len(request.Headers))
		for k, vs := range request.Headers {
			tmp := make([]string, len(vs))
			copy(tmp, vs)
			requestToBakend.Header[k] = tmp
		}
		if request.Body != nil {
			if v, ok := request.Headers["Content-Length"]; ok && len(v) == 1 && v[0] != "chunked" {
				if size, err := strconv.Atoi(v[0]); err == nil {
					requestToBakend.ContentLength = int64(size)
				}
			}
		}

		resp, err := re(ctx, requestToBakend)
		if requestToBakend.Body != nil {
			requestToBakend.Body.Close()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if err != nil {
			return nil, err
		}

		resp, err = ch(ctx, resp)
		if err != nil {
			if t, ok := err.(responseError); ok {
				return &Response{
					Data: map[string]interface{}{
						fmt.Sprintf("error_%s", t.Name()): t,
					},
					Metadata: Metadata{StatusCode: t.StatusCode()},
				}, nil
			}
			return nil, err
		}

		return rp(ctx, resp)
	}
}

// NewRequestBuilderMiddleware creates a proxy middleware that parses the request params received
// from the outter layer and generates the path to the backend endpoints
func NewRequestBuilderMiddleware(remote *config.Backend) Middleware {
	return func(next ...Proxy) Proxy {
		if len(next) > 1 {
			panic(ErrTooManyProxies)
		}
		return func(ctx context.Context, request *Request) (*Response, error) {
			r := request.Clone()
			r.GeneratePath(remote.URLPattern)
			r.Method = remote.Method
			return next[0](ctx, &r)
		}
	}
}

type responseError interface {
	Error() string
	Name() string
	StatusCode() int
}
