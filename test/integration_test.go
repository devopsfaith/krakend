// +build integration !race

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/urfave/negroni"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/logging"
	"github.com/devopsfaith/krakend/proxy"
	"github.com/devopsfaith/krakend/router/chi"
	"github.com/devopsfaith/krakend/router/gin"
	"github.com/devopsfaith/krakend/router/gorilla"
	"github.com/devopsfaith/krakend/router/httptreemux"
	krakendnegroni "github.com/devopsfaith/krakend/router/negroni"
)

func TestKrakenD_ginRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testKrakenD(t, func(logger logging.Logger, cfg *config.ServiceConfig) {
		gin.DefaultFactory(proxy.DefaultFactory(logger), logger).NewWithContext(ctx).Run(*cfg)
	})
}

func TestKrakenD_gorillaRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config.RoutingPattern = config.BracketsRouterPatternBuilder
	testKrakenD(t, func(logger logging.Logger, cfg *config.ServiceConfig) {
		gorilla.DefaultFactory(proxy.DefaultFactory(logger), logger).NewWithContext(ctx).Run(*cfg)
	})
	config.RoutingPattern = config.ColonRouterPatternBuilder
}

func TestKrakenD_negroniRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config.RoutingPattern = config.BracketsRouterPatternBuilder
	testKrakenD(t, func(logger logging.Logger, cfg *config.ServiceConfig) {
		factory := krakendnegroni.DefaultFactory(proxy.DefaultFactory(logger), logger, []negroni.Handler{})
		factory.NewWithContext(ctx).Run(*cfg)
	})
	config.RoutingPattern = config.ColonRouterPatternBuilder
}

func TestKrakenD_httptreemuxRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testKrakenD(t, func(logger logging.Logger, cfg *config.ServiceConfig) {
		httptreemux.DefaultFactory(proxy.DefaultFactory(logger), logger).NewWithContext(ctx).Run(*cfg)
	})
}

func TestKrakenD_chiRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config.RoutingPattern = config.BracketsRouterPatternBuilder
	testKrakenD(t, func(logger logging.Logger, cfg *config.ServiceConfig) {
		chi.DefaultFactory(proxy.DefaultFactory(logger), logger).NewWithContext(ctx).Run(*cfg)
	})
	config.RoutingPattern = config.ColonRouterPatternBuilder
}

func testKrakenD(t *testing.T, runRouter func(logging.Logger, *config.ServiceConfig)) {
	cfg, err := setupBackend(t)
	if err != nil {
		t.Error(err)
		return
	}

	buf := new(bytes.Buffer)
	logger, err := logging.NewLogger("DEBUG", buf, "[KRAKEND]")
	if err != nil {
		t.Error(err)
		return
	}

	go runRouter(logger, cfg)

	select {
	case <-time.After(300 * time.Millisecond):
	}

	defaultHeaders := map[string]string{
		"Content-Type":        "application/json",
		"X-KrakenD-Completed": "true",
		"X-Krakend":           "Version undefined",
	}

	incompleteHeader := map[string]string{
		"Content-Type":        "application/json",
		"X-KrakenD-Completed": "false",
		"X-Krakend":           "Version undefined",
	}

	for _, tc := range []struct {
		name          string
		url           string
		method        string
		headers       map[string]string
		body          string
		expBody       string
		expHeaders    map[string]string
		expStatusCode int
	}{
		{
			name:       "static",
			url:        "/static",
			headers:    map[string]string{},
			expHeaders: incompleteHeader,
			expBody:    `{"bar":"foobar","foo":42}`,
		},
		{
			name:   "param_forwarding",
			url:    "/param_forwarding/foo/constant/bar",
			method: "POST",
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "bearer AuthorizationToken",
				"X-Y-Z":         "x-y-z",
			},
			body:       `{"foo":"bar"}`,
			expHeaders: defaultHeaders,
			expBody:    `{"path":"/foo/bar"}`,
		},
		{
			name:   "param_forwarding_2",
			url:    "/param_forwarding/foo/constant/foobar",
			method: "POST",
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "bearer AuthorizationToken",
				"X-Y-Z":         "x-y-z",
			},
			body:       `{"foo":"bar"}`,
			expHeaders: defaultHeaders,
			expBody:    `{"path":"/foo/foobar"}`,
		},
		{
			name:       "timeout",
			url:        "/timeout",
			headers:    map[string]string{},
			expHeaders: incompleteHeader,
			expBody:    `{"email":"some@email.com","name":"a"}`,
		},
		{
			name:       "partial_with_static",
			url:        "/partial/static",
			headers:    map[string]string{},
			expHeaders: incompleteHeader,
			expBody:    `{"bar":"foobar","email":"some@email.com","foo":42,"name":"a"}`,
		},
		{
			name:       "partial",
			url:        "/partial",
			headers:    map[string]string{},
			expHeaders: incompleteHeader,
			expBody:    `{"email":"some@email.com","name":"a"}`,
		},
		{
			name:       "combination",
			url:        "/combination",
			headers:    map[string]string{},
			expHeaders: defaultHeaders,
			expBody:    `{"name":"a","personal_email":"some@email.com","posts":[{"body":"some content","date":"123456789"},{"body":"some other content","date":"123496789"}]}`,
		},
		{
			name:       "detail_error",
			url:        "/detail_error",
			headers:    map[string]string{},
			expHeaders: incompleteHeader,
			expBody:    `{"email":"some@email.com","error_backend_a":{"http_status_code":429,"http_body":"sad panda\n"},"name":"a"}`,
		},
		{
			name:       "querystring-params-no-params",
			url:        "/querystring-params-test/no-params?a=1&b=2&c=3",
			headers:    map[string]string{},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Version undefined"]},"path":"/no-params","query":{}}`,
		},
		{
			name:       "querystring-params-optional-query-params",
			url:        "/querystring-params-test/query-params?a=1&b=2&c=3",
			headers:    map[string]string{},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Version undefined"]},"path":"/query-params","query":{"a":["1"],"b":["2"]}}`,
		},
		{
			name:       "querystring-params-mandatory-query-params",
			url:        "/querystring-params-test/url-params/some?a=1&b=2&c=3",
			headers:    map[string]string{},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Version undefined"]},"path":"/url-params","query":{"p":["some"]}}`,
		},
		{
			name:       "querystring-params-all",
			url:        "/querystring-params-test/all-params?a=1&b=2&c=3",
			headers:    map[string]string{},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Version undefined"]},"path":"/all-params","query":{"a":["1"],"b":["2"],"c":["3"]}}`,
		},
		{
			name: "header-params-none",
			url:  "/header-params-test/no-params",
			headers: map[string]string{
				"x-Test-1": "some",
				"X-TEST-2": "none",
			},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Version undefined"]},"path":"/no-params","query":{}}`,
		},
		{
			name: "header-params-filter",
			url:  "/header-params-test/filter-params",
			headers: map[string]string{
				"x-tESt-1": "some",
				"X-TEST-2": "none",
			},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Version undefined"],"X-Test-1":["some"]},"path":"/filter-params","query":{}}`,
		},
		{
			name: "header-params-all",
			url:  "/header-params-test/all-params",
			headers: map[string]string{
				"x-Test-1":   "some",
				"X-TEST-2":   "none",
				"User-Agent": "KrakenD Test",
			},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Test"],"X-Forwarded-Via":["KrakenD Version undefined"],"X-Test-1":["some"],"X-Test-2":["none"]},"path":"/all-params","query":{}}`,
		},
		{
			name:       "sequential ok",
			url:        "/sequential/ok/foo",
			expHeaders: defaultHeaders,
			expBody:    `{"path":"/recipient/42","random":42}`,
		},
		{
			name: "sequential ko first",
			url:  "/sequential/ko/first/foo",
			expHeaders: map[string]string{
				"X-KrakenD-Completed": "false",
				"X-Krakend":           "Version undefined",
			},
			expStatusCode: 500,
		},
		{
			name:       "sequential ko last",
			url:        "/sequential/ko/last/foo",
			expHeaders: incompleteHeader,
			expBody:    `{"random":42}`,
		},
		{
			name:       "redirect",
			url:        "/redirect",
			expHeaders: defaultHeaders,
			expBody:    `{"path":"/","random":42}`,
		},
		{
			name:       "found",
			url:        "/found",
			expHeaders: defaultHeaders,
			expBody:    `{"path":"/","random":42}`,
		},
		{
			name:       "flatmap del",
			url:        "/flatmap/delete",
			expHeaders: defaultHeaders,
			expBody:    `{"collection":[{"body":"some content"},{"body":"some other content"}]}`,
		},
		{
			name:       "flatmap rename",
			url:        "/flatmap/rename",
			expHeaders: defaultHeaders,
			expBody:    `{"collection":[{"body":"some content","created_at":"123456789"},{"body":"some other content","created_at":"123496789"}]}`,
		},
		{
			name: "x-forwarded-for",
			url:  "/x-forwarded-for",
			headers: map[string]string{
				"x-forwarded-for": "123.45.67.89",
			},
			expHeaders: defaultHeaders,
			expBody:    `{"headers":{"Accept-Encoding":["gzip"],"User-Agent":["KrakenD Version undefined"],"X-Forwarded-For":["123.45.67.89"]}}`,
		},
		{
			name:          "required backend request",
			url:           "/required_backend",
			expHeaders:    incompleteHeader,
			expStatusCode: 422,
			expBody:       `{"data":"some err"}`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.method == "" {
				tc.method = "GET"
			}

			var body io.Reader
			if tc.body != "" {
				body = bytes.NewBufferString(tc.body)
			}

			url := fmt.Sprintf("http://localhost:%d%s", cfg.Port, tc.url)

			r, _ := http.NewRequest(tc.method, url, body)
			for k, v := range tc.headers {
				r.Header.Add(k, v)
			}

			resp, err := http.DefaultClient.Do(r)
			if err != nil {
				t.Error(err)
				return
			}
			if resp == nil {
				t.Errorf("%s: nil response", resp.Request.URL.Path)
				return
			}
			expectedStatusCode := http.StatusOK
			if tc.expStatusCode != 0 {
				expectedStatusCode = tc.expStatusCode
			}
			if resp.StatusCode != expectedStatusCode {
				t.Errorf("%s: unexpected status code. have: %d, want: %d", resp.Request.URL.Path, resp.StatusCode, expectedStatusCode)
			}

			for k, v := range tc.expHeaders {
				if c := resp.Header.Get(k); !strings.Contains(c, v) {
					t.Errorf("%s: unexpected header %s: %s", resp.Request.URL.Path, k, c)
				}
			}
			if tc.expBody == "" {
				return
			}

			b, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if tc.expBody != string(b) {
				t.Errorf("%s: unexpected body: %s", resp.Request.URL.Path, string(b))
				fmt.Println(resp.Request.URL.Path, "was expecting:", tc.expBody)
			}
		})
	}

}

func setupBackend(t *testing.T) (*config.ServiceConfig, error) {
	data := map[string]interface{}{"port": rand.Intn(2000) + 8080}

	// param forwarding validation backend
	b1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if c := r.Header.Get("Content-Type"); c != "application/json" {
			t.Errorf("unexpected header content-type: %s", c)
			http.Error(rw, "bad content-type", 400)
			return
		}
		if c := r.Header.Get("Authorization"); c != "bearer AuthorizationToken" {
			t.Errorf("unexpected header Authorization: %s", c)
			http.Error(rw, "bad Authorization", 400)
			return
		}
		if c := r.Header.Get("X-Y-Z"); c != "x-y-z" {
			t.Errorf("unexpected header X-Y-Z: %s", c)
			http.Error(rw, "bad X-Y-Z", 400)
			return
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if string(body) != `{"foo":"bar"}` {
			t.Errorf("unexpected request body: %s", string(body))
			return
		}
		rw.Header().Add("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]interface{}{"path": r.URL.Path})
	}))
	data["b1"] = b1.URL

	// collection generator backend
	b2 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "application/json")
		json.NewEncoder(rw).Encode([]interface{}{
			map[string]interface{}{"body": "some content", "date": "123456789"},
			map[string]interface{}{"body": "some other content", "date": "123496789"},
		})
	}))
	data["b2"] = b2.URL

	// regular struct generator backend
	b3 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]interface{}{"email": "some@email.com", "name": "a"})
	}))
	data["b3"] = b3.URL

	// crasher backend
	b4 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Error(rw, "sad panda", 429)
	}))
	data["b4"] = b4.URL

	// slow backend
	b5 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		<-time.After(time.Second)
		rw.Header().Add("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]interface{}{"email": "some@email.com", "name": "a"})
	}))
	data["b5"] = b5.URL

	// querystring-forwarding backend
	b6 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "application/json")
		if ip := net.ParseIP(r.Header.Get("X-Forwarded-For")); ip == nil || !ip.IsLoopback() {
			http.Error(rw, "invalid X-Forwarded-For", 400)
			return
		}
		r.Header.Del("X-Forwarded-For")
		json.NewEncoder(rw).Encode(map[string]interface{}{
			"path":    r.URL.Path,
			"query":   r.URL.Query(),
			"headers": r.Header,
		})
	}))
	data["b6"] = b6.URL

	// path validator
	b7 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]interface{}{"path": r.URL.Path, "random": 42})
	}))
	data["b7"] = b7.URL

	// redirect
	b8 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, b7.URL, http.StatusMovedPermanently)
	}))
	data["b8"] = b8.URL

	// found
	b9 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, b7.URL, http.StatusFound)
	}))
	data["b9"] = b9.URL

	// X-Forwarded-For backend
	b11 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Add("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]interface{}{
			"headers": r.Header,
		})
	}))
	data["b11"] = b11.URL

	// some err
	b12 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprintln(rw, `{"data":"some err"}`)
	}))
	data["b12"] = b12.URL

	c, err := loadConfig(data)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func loadConfig(data map[string]interface{}) (*config.ServiceConfig, error) {
	content, _ := ioutil.ReadFile("krakend.json")
	tmpl, err := template.New("test").Parse(string(content))
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err = tmpl.Execute(buf, data); err != nil {
		return nil, err
	}

	c, err := config.NewParserWithFileReader(func(s string) ([]byte, error) {
		return []byte(s), nil
	}).Parse(buf.String())

	if err != nil {
		return nil, err
	}

	return &c, nil
}
