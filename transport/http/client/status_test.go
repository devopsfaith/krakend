// SPDX-License-Identifier: Apache-2.0
package client

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/luraproject/lura/config"
)

func TestDetailedHTTPStatusHandler(t *testing.T) {
	expectedErrName := "some"
	cfg := &config.Backend{
		ExtraConfig: config.ExtraConfig{
			Namespace: map[string]interface{}{
				"return_error_details": expectedErrName,
			},
		},
	}
	sh := GetHTTPStatusHandler(cfg)

	for _, code := range []int{http.StatusOK, http.StatusCreated,
		http.StatusAccepted, http.StatusNonAuthoritativeInfo, http.StatusNoContent,
		http.StatusResetContent} {
		resp := &http.Response{
			StatusCode: code,
			Body:       ioutil.NopCloser(bytes.NewBufferString(`{"foo":"bar"}`)),
		}

		r, err := sh(context.Background(), resp)

		if r != resp {
			t.Errorf("#%d unexpected response: %v", code, r)
			return
		}

		if err != nil {
			t.Errorf("#%d unexpected error: %s", code, err.Error())
			return
		}
	}

	for i, code := range statusCodes {
		msg := http.StatusText(code)

		resp := &http.Response{
			StatusCode: code,
			Body:       ioutil.NopCloser(bytes.NewBufferString(msg)),
		}

		r, err := sh(context.Background(), resp)

		if r != resp {
			t.Errorf("#%d unexpected response: %v", i, r)
			return
		}

		e, ok := err.(HTTPResponseError)
		if !ok {
			t.Errorf("#%d unexpected error type %T: %s", i, err, err.Error())
			return
		}

		if e.StatusCode() != code {
			t.Errorf("#%d unexpected status code: %d", i, e.Code)
			return
		}

		if e.Error() != msg {
			t.Errorf("#%d unexpected message: %s", i, e.Msg)
			return
		}

		if e.Name() != expectedErrName {
			t.Errorf("#%d unexpected error name: %s", i, e.name)
			return
		}
	}
}

func TestDefaultHTTPStatusHandler(t *testing.T) {
	sh := GetHTTPStatusHandler(&config.Backend{})

	for _, code := range []int{http.StatusOK, http.StatusCreated,
		http.StatusAccepted, http.StatusNonAuthoritativeInfo, http.StatusNoContent,
		http.StatusResetContent} {
		resp := &http.Response{
			StatusCode: code,
			Body:       ioutil.NopCloser(bytes.NewBufferString(`{"foo":"bar"}`)),
		}

		r, err := sh(context.Background(), resp)

		if r != resp {
			t.Errorf("#%d unexpected response: %v", code, r)
			return
		}

		if err != nil {
			t.Errorf("#%d unexpected error: %s", code, err.Error())
			return
		}
	}

	for _, code := range statusCodes {
		msg := http.StatusText(code)

		resp := &http.Response{
			StatusCode: code,
			Body:       ioutil.NopCloser(bytes.NewBufferString(msg)),
		}

		r, err := sh(context.Background(), resp)

		if r != nil {
			t.Errorf("#%d unexpected response: %v", code, r)
			return
		}

		if err != ErrInvalidStatusCode {
			t.Errorf("#%d unexpected error: %v", code, err)
			return
		}
	}
}

var statusCodes = []int{
	http.StatusBadRequest,
	http.StatusUnauthorized,
	http.StatusPaymentRequired,
	http.StatusForbidden,
	http.StatusNotFound,
	http.StatusMethodNotAllowed,
	http.StatusNotAcceptable,
	http.StatusProxyAuthRequired,
	http.StatusRequestTimeout,
	http.StatusConflict,
	http.StatusGone,
	http.StatusLengthRequired,
	http.StatusPreconditionFailed,
	http.StatusRequestEntityTooLarge,
	http.StatusRequestURITooLong,
	http.StatusUnsupportedMediaType,
	http.StatusRequestedRangeNotSatisfiable,
	http.StatusExpectationFailed,
	http.StatusTeapot,
	// http.StatusMisdirectedRequest,
	http.StatusUnprocessableEntity,
	http.StatusLocked,
	http.StatusFailedDependency,
	http.StatusUpgradeRequired,
	http.StatusPreconditionRequired,
	http.StatusTooManyRequests,
	http.StatusRequestHeaderFieldsTooLarge,
	http.StatusUnavailableForLegalReasons,

	http.StatusInternalServerError,
	http.StatusNotImplemented,
	http.StatusBadGateway,
	http.StatusServiceUnavailable,
	http.StatusGatewayTimeout,
	http.StatusHTTPVersionNotSupported,
	http.StatusVariantAlsoNegotiates,
	http.StatusInsufficientStorage,
	http.StatusLoopDetected,
	http.StatusNotExtended,
	http.StatusNetworkAuthenticationRequired,
}
