package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/encoding"
	"github.com/devopsfaith/krakend/transport/http/client"
)

func TestNewHTTPProxy_ok(t *testing.T) {
	expectedMethod := "GET"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength != 11 {
			t.Errorf("unexpected request size. Want: 11. Have: %d", r.ContentLength)
		}
		if h := r.Header.Get("Content-Length"); h != "11" {
			t.Errorf("unexpected content-length header. Want: 11. Have: %s", h)
		}
		if r.Method != expectedMethod {
			t.Errorf("Wrong request method. Want: %s. Have: %s", expectedMethod, r.Method)
		}
		if h := r.Header.Get("X-First"); h != "first" {
			t.Errorf("unexpected first header: %s", h)
		}
		if h := r.Header.Get("X-Second"); h != "second" {
			t.Errorf("unexpected second header: %s", h)
		}
		r.Header.Del("X-Second")
		fmt.Fprintf(w, "{\"supu\":42, \"tupu\":true, \"foo\": \"bar\"}")
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: encoding.JSONDecoder,
	}
	request := Request{
		Method: expectedMethod,
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(`{"abc": 42}`),
		Headers: map[string][]string{
			"X-First":        {"first"},
			"X-Second":       {"second"},
			"Content-Length": {"11"},
		},
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	result, err := HTTPProxyFactory(http.DefaultClient)(&backend)(context.Background(), &request)
	if err != nil {
		t.Errorf("The proxy returned an unexpected error: %s\n", err.Error())
		return
	}
	if result == nil {
		t.Errorf("The proxy returned a null result\n")
		return
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
		return
	default:
	}

	tmp, ok := result.Data["supu"]
	if !ok {
		t.Errorf("The proxy returned an unexpected result: %v\n", result)
	}
	supuValue, err := tmp.(json.Number).Int64()
	if err != nil || supuValue != 42 {
		t.Errorf("The proxy returned an unexpected result: %v\n", supuValue)
	}
	if v, ok := result.Data["tupu"]; !ok || !v.(bool) {
		t.Errorf("The proxy returned an unexpected result: %v\n", result)
	}
	if v, ok := result.Data["foo"]; !ok || v.(string) != "bar" {
		t.Errorf("The proxy returned an unexpected result: %v\n", result)
	}
	if v, ok := request.Headers["X-Second"]; !ok || len(v) != 1 {
		t.Errorf("the proxy request headers were changed: %v", request.Headers)
	}
}

func TestNewHTTPProxy_cancel(t *testing.T) {
	expectedMethod := "GET"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(300) * time.Millisecond)
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: encoding.JSONDecoder,
	}
	request := Request{
		Method: expectedMethod,
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Millisecond)
	defer cancel()
	response, err := httpProxy(&backend)(ctx, &request)
	if err == nil || err.Error() != "context deadline exceeded" {
		t.Errorf("The proxy didn't propagate a timeout error: %s\n", err)
	}
	if response != nil {
		t.Errorf("We weren't expecting a response but we got one: %v\n", response)
		return
	}
	select {
	case <-mustEnd:
		t.Errorf("We were expecting a response at this point in time!\n")
		return
	default:
	}
}

func TestNewHTTPProxy_badResponseBody(t *testing.T) {
	expectedMethod := "GET"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "supu")
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: encoding.JSONDecoder,
	}
	request := Request{
		Method: expectedMethod,
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Millisecond)
	defer cancel()
	response, err := httpProxy(&backend)(ctx, &request)
	if err == nil || err.Error() != "invalid character 's' looking for beginning of value" {
		t.Errorf("The proxy didn't propagate the backend error: %s\n", err)
	}
	if response != nil {
		t.Errorf("We weren't expecting a response but we got one: %v\n", response)
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
	default:
	}
}

func TestNewHTTPProxy_badStatusCode(t *testing.T) {
	expectedMethod := "GET"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "booom", 500)
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: encoding.JSONDecoder,
	}
	request := Request{
		Method: expectedMethod,
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Millisecond)
	defer cancel()
	response, err := httpProxy(&backend)(ctx, &request)
	if err == nil || err != client.ErrInvalidStatusCode {
		t.Errorf("The proxy didn't propagate the backend error: %s\n", err)
	}
	if response != nil {
		t.Errorf("We weren't expecting a response but we got one: %v\n", response)
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
	default:
	}
}

func TestNewHTTPProxy_badStatusCode_detailed(t *testing.T) {
	expectedMethod := "GET"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "booom", 500)
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: encoding.JSONDecoder,
		ExtraConfig: config.ExtraConfig{
			client.Namespace: map[string]interface{}{
				"return_error_details": "some",
			},
		},
	}
	request := Request{
		Method: expectedMethod,
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Millisecond)
	defer cancel()
	response, err := httpProxy(&backend)(ctx, &request)
	if err != nil {
		t.Errorf("The proxy propagated the backend error: %s", err.Error())
	}
	if response == nil {
		t.Error("We were expecting a response but we got none")
		return
	}
	if response.Metadata.StatusCode != 500 {
		t.Errorf("unexpected error code: %d", response.Metadata.StatusCode)
	}
	b, _ := json.Marshal(response.Data)
	if string(b) != `{"error_some":{"http_status_code":500,"http_body":"booom\n"}}` {
		t.Errorf("unexpected response content: %s", string(b))
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
	default:
	}
}

func TestNewHTTPProxy_StatusCode_can_be_unchecked(t *testing.T) {
	expectedMethod := "PUT"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"id": 1}`, http.StatusAccepted)
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: encoding.JSONDecoder,
		StatusCodeUnchecked: "yes",
		StatusCodeDictator: true,
	}
	request := Request{
		Method: expectedMethod,
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Hour)
	defer cancel()
	response, err := httpProxy(&backend)(ctx, &request)
	if err != nil {
		t.Errorf("The proxy propagated the backend error: %s", err.Error())
	}
	if response == nil {
		t.Error("We were expecting a response but we got none")
		return
	}
	if response.Metadata.StatusCode != http.StatusAccepted {
		t.Errorf("unexpected status code: %d", response.Metadata.StatusCode)
	}
	b, _ := json.Marshal(response.Data)
	if string(b) != `{"id":1}` {
		t.Errorf("unexpected response content: %s", string(b))
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
	default:
	}
}

func TestNewHTTPProxy_decodingError(t *testing.T) {
	expectedMethod := "GET"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"supu": 42}`)
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: func(_ io.Reader, _ *map[string]interface{}) error {
			return errors.New("booom")
		},
	}
	request := Request{
		Method: expectedMethod,
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Millisecond)
	defer cancel()
	response, err := httpProxy(&backend)(ctx, &request)
	if err == nil || err.Error() != "booom" {
		t.Errorf("The proxy returned an unexpected error: %s\n", err.Error())
	}
	if response != nil {
		t.Errorf("We weren't expecting a response but we got one: %v\n", response)
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
	default:
	}
}

func TestNewHTTPProxy_badMethod(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("The handler shouldn't be called")
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: func(_ io.Reader, _ *map[string]interface{}) error {
			t.Error("The decoder shouldn't be called")
			return nil
		},
	}
	request := Request{
		Method: "\n",
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Millisecond)
	defer cancel()
	_, err := httpProxy(&backend)(ctx, &request)
	if err == nil {
		t.Error("The proxy didn't return the expected error")
		return
	}
	if err.Error() != "net/http: invalid method \"\\n\"" {
		t.Errorf("The proxy returned an unexpected error: %s\n", err.Error())
		return
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
	default:
	}
}

func TestNewHTTPProxy_requestKo(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("The handler shouldn't be called")
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Decoder: func(_ io.Reader, _ *map[string]interface{}) error {
			t.Error("The decoder shouldn't be called")
			return nil
		},
	}
	request := Request{
		Method: "GET",
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Millisecond)
	defer cancel()

	expectedError := fmt.Errorf("MAYDAY, MAYDAY")
	_, err := NewHTTPProxyWithHTTPExecutor(&backend, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return nil, expectedError
	}, backend.Decoder)(ctx, &request)
	if err == nil {
		t.Error("The proxy didn't return the expected error")
		return
	}
	if err != expectedError {
		t.Errorf("The proxy returned an unexpected error: %s\n", err.Error())
		return
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
	default:
	}
}

func TestNewRequestBuilderMiddleware_ok(t *testing.T) {
	expected := errors.New("error to be propagated")
	expectedMethod := "GET"
	expectedPath := "/supu"
	assertion := func(ctx context.Context, request *Request) (*Response, error) {
		if request.Method != expectedMethod {
			err := fmt.Errorf("Wrong request method. Want: %s. Have: %s", expectedMethod, request.Method)
			t.Errorf(err.Error())
			return nil, err
		}
		if request.Path != expectedPath {
			err := fmt.Errorf("Wrong request path. Want: %s. Have: %s", expectedPath, request.Path)
			t.Errorf(err.Error())
			return nil, err
		}
		return nil, expected
	}
	sampleBackend := config.Backend{
		URLPattern: expectedPath,
		Method:     expectedMethod,
	}
	mw := NewRequestBuilderMiddleware(&sampleBackend)
	response, err := mw(assertion)(context.Background(), &Request{})
	if err != expected {
		t.Errorf("The middleware propagated an unexpected error: %s\n", err.Error())
	}
	if response != nil {
		t.Errorf("We weren't expecting a response but we got one: %v\n", response)
	}
}

func TestNewRequestBuilderMiddleware_multipleNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic\n")
		}
	}()
	sampleBackend := config.Backend{}
	mw := NewRequestBuilderMiddleware(&sampleBackend)
	mw(explosiveProxy(t), explosiveProxy(t))
}

func TestDefaultHTTPResponseParserConfig_nopDecoder(t *testing.T) {
	result := map[string]interface{}{}
	if err := DefaultHTTPResponseParserConfig.Decoder(bytes.NewBufferString("some body"), &result); err != nil {
		t.Error(err.Error())
	}
	if len(result) != 0 {
		t.Error("unexpected result")
	}
}

func TestDefaultHTTPResponseParserConfig_nopEntityFormatter(t *testing.T) {
	expected := Response{Data: map[string]interface{}{"supu": "tupu"}, IsComplete: true}
	result := DefaultHTTPResponseParserConfig.EntityFormatter.Format(expected)
	if !result.IsComplete {
		t.Error("unexpected result")
	}
	d, ok := result.Data["supu"]
	if !ok {
		t.Error("unexpected result")
	}
	if v, ok := d.(string); !ok || v != "tupu" {
		t.Error("unexpected result")
	}
}

func TestNewHTTPProxy_noopDecoder(t *testing.T) {
	expectedcontent := "some nice, interesting and long content"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("header1", "value1")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedcontent))
	}))
	defer backendServer.Close()

	rpURL, _ := url.Parse(backendServer.URL)
	backend := config.Backend{
		Encoding: encoding.NOOP,
		Decoder:  encoding.NoOpDecoder,
	}
	request := Request{
		Method: "GET",
		Path:   "/",
		URL:    rpURL,
		Body:   newDummyReadCloser(""),
	}
	mustEnd := time.After(time.Duration(150) * time.Millisecond)

	result, err := HTTPProxyFactory(http.DefaultClient)(&backend)(context.Background(), &request)
	if err != nil {
		t.Errorf("The proxy returned an unexpected error: %s\n", err.Error())
		return
	}
	if result == nil {
		t.Errorf("The proxy returned a null result\n")
		return
	}
	select {
	case <-mustEnd:
		t.Errorf("Error: expected response")
		return
	default:
	}

	if len(result.Data) > 0 {
		t.Error("unexpected data:", result.Data)
		return
	}

	if result.Metadata.StatusCode != http.StatusOK {
		t.Error("unexpected status code:", result.Metadata.StatusCode)
		return
	}

	if len(result.Metadata.Headers["Header1"]) < 1 || result.Metadata.Headers["Header1"][0] != "value1" {
		t.Error("unexpected header:", result.Metadata.Headers)
		return
	}

	b := &bytes.Buffer{}
	if _, err := b.ReadFrom(result.Io); err != nil {
		t.Error(err, b.String())
		return
	}
	if content := b.String(); content != expectedcontent {
		t.Error("unexpected content:", content)
	}
}
