package proxy

import (
	"context"
	"io"
	"net/http"

	"github.com/devopsfaith/krakend/encoding"
)

// HTTPResponseParser defines how the response is parsed from http.Response to Response object
type HTTPResponseParser func(context.Context, *http.Response) (*Response, error)

// DefaultHTTPResponseParserConfig defines a default HTTPResponseParserConfig
var DefaultHTTPResponseParserConfig = HTTPResponseParserConfig{
	func(_ io.Reader, _ *map[string]interface{}) error { return nil },
	EntityFormatterFunc(func(r Response) Response { return r }),
}

// HTTPResponseParserConfig contains the config for a given HttpResponseParser
type HTTPResponseParserConfig struct {
	Decoder         encoding.Decoder
	EntityFormatter EntityFormatter
}

// HTTPResponseParserFactory creates HTTPResponseParser from a given HTTPResponseParserConfig
type HTTPResponseParserFactory func(HTTPResponseParserConfig) HTTPResponseParser

// DefaultHTTPResponseParserFactory is the default implementation of HTTPResponseParserFactory
func DefaultHTTPResponseParserFactory(cfg HTTPResponseParserConfig) HTTPResponseParser {
	return func(ctx context.Context, resp *http.Response) (*Response, error) {
		var data map[string]interface{}
		err := cfg.Decoder(resp.Body, &data)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		newResponse := Response{Data: data, IsComplete: true}
		newResponse = cfg.EntityFormatter.Format(newResponse)
		return &newResponse, nil
	}
}

// HTTPResponseWithErrorParserFactory is the default implementation of HTTPResponseParserFactory
func HTTPResponseWithErrorParserFactory(cfg HTTPResponseParserConfig) HTTPResponseParser {
	return func(ctx context.Context, resp *http.Response) (*Response, error) {
		var data map[string]interface{}
		err := cfg.Decoder(resp.Body, &data)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		newResponse := Response{Data: data, IsComplete: true}
		if resp.StatusCode >= 400 {
			newResponse.Metadata.StatusCode = resp.StatusCode
			newResponse.IsComplete = false
			newResponse.Metadata.IsRequired = true
		}
		newResponse = cfg.EntityFormatter.Format(newResponse)
		return &newResponse, nil
	}
}

// NoOpHTTPResponseParser is a HTTPResponseParser implementation that just copies the
// http response body into the proxy response IO
func NoOpHTTPResponseParser(ctx context.Context, resp *http.Response) (*Response, error) {
	return &Response{
		Data:       map[string]interface{}{},
		IsComplete: true,
		Io:         NewReadCloserWrapper(ctx, resp.Body),
		Metadata: Metadata{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
		},
	}, nil
}
