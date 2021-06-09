// SPDX-License-Identifier: Apache-2.0
package client

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/luraproject/lura/config"
)

// Namespace to be used in extra config
const Namespace = "github.com/devopsfaith/krakend/http"

// ErrInvalidStatusCode is the error returned by the http proxy when the received status code
// is not a successful http status code
var ErrInvalidStatusCode = errors.New("Invalid status code")

// HTTPStatusHandler defines how we tread the http response code
type HTTPStatusHandler func(context.Context, *http.Response) (*http.Response, error)

// GetHTTPStatusHandler returns a status handler. If the 'return_error_details' key is defined
// at the extra config, it returns a DetailedHTTPStatusHandler. Otherwise, it returns a
// DefaultHTTPStatusHandler
func GetHTTPStatusHandler(remote *config.Backend) HTTPStatusHandler {
	if e, ok := remote.ExtraConfig[Namespace]; ok {
		if m, ok := e.(map[string]interface{}); ok {
			if v, ok := m["return_error_details"]; ok {
				if b, ok := v.(string); ok && b != "" {
					return DetailedHTTPStatusHandler(DefaultHTTPStatusHandler, b)
				}
			}
		}
	}
	return DefaultHTTPStatusHandler
}

// DefaultHTTPStatusHandler is the default implementation of HTTPStatusHandler
func DefaultHTTPStatusHandler(ctx context.Context, resp *http.Response) (*http.Response, error) {
	if !isSuccessResponse(resp.StatusCode) {
		return nil, ErrInvalidStatusCode
	}

	return resp, nil
}

// NoOpHTTPStatusHandler is a NO-OP implementation of HTTPStatusHandler
func NoOpHTTPStatusHandler(_ context.Context, resp *http.Response) (*http.Response, error) {
	return resp, nil
}

// DetailedHTTPStatusHandler is a HTTPStatusHandler implementation
func DetailedHTTPStatusHandler(next HTTPStatusHandler, name string) HTTPStatusHandler {
	return func(ctx context.Context, resp *http.Response) (*http.Response, error) {
		if r, err := next(ctx, resp); err == nil {
			return r, nil
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			body = []byte{}
		}
		resp.Body.Close()
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		return resp, HTTPResponseError{
			Code: resp.StatusCode,
			Msg:  string(body),
			name: name,
		}
	}
}

// HTTPResponseError is the error to be returned by the DetailedHTTPStatusHandler
type HTTPResponseError struct {
	Code int    `json:"http_status_code"`
	Msg  string `json:"http_body,omitempty"`
	name string
}

// Error returns the error message
func (r HTTPResponseError) Error() string {
	return r.Msg
}

// Name returns the name of the error
func (r HTTPResponseError) Name() string {
	return r.name
}

// StatusCode returns the status code returned by the backend
func (r HTTPResponseError) StatusCode() int {
	return r.Code
}

func isSuccessResponse(statusCode int) bool {
	switch statusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted,
		http.StatusNonAuthoritativeInfo, http.StatusNoContent,
		http.StatusResetContent:
		return true
	}
	return false
}
