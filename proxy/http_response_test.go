package proxy

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNopHTTPResponseParser(t *testing.T) {
	w := httptest.NewRecorder()
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("header1", "value1")
		w.Write([]byte("some nice, interesting and long content"))
	}
	req, _ := http.NewRequest("GET", "/url", nil)
	handler(w, req)
	result, err := NoOpHTTPResponseParser(context.Background(), w.Result())
	if !result.IsComplete {
		t.Error("unexpected result")
	}
	if len(result.Data) != 0 {
		t.Error("unexpected result")
	}
	if result.Metadata.StatusCode != http.StatusOK {
		t.Error("unexpected result")
	}
	headers := result.Metadata.Headers
	if h, ok := headers["Header1"]; !ok || h[0] != "value1" {
		t.Error("unexpected result:", result.Metadata.Headers)
	}
	body, err := ioutil.ReadAll(result.Io)
	if err != nil {
		t.Error("unexpected error:", err.Error())
	}
	if string(body) != "some nice, interesting and long content" {
		t.Error("unexpected result")
	}
}

func TestHTTPResponseWithErrorParserFactory(t *testing.T) {
	type args struct {
		cfg        HTTPResponseParserConfig
		statusCode int
	}
	tests := []struct {
		name           string
		args           args
		wantMetaData   Metadata
		wantStatusCode int
		wantIsComplete bool
	}{
		{
			name: "success bad status",
			args: args{
				cfg:        DefaultHTTPResponseParserConfig,
				statusCode: http.StatusBadGateway,
			},
			wantMetaData: Metadata{
				StatusCode: 0,
				IsRequired: true,
			},
			wantIsComplete: false,
			wantStatusCode: http.StatusBadGateway,
		},
		{
			name: "success status ok",
			args: args{
				cfg:        DefaultHTTPResponseParserConfig,
				statusCode: http.StatusOK,
			},
			wantMetaData: Metadata{
				StatusCode: 0,
				IsRequired: false,
			},
			wantIsComplete: true,
			wantStatusCode: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			w := httptest.NewRecorder()
			handler := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("header1", "value1")
				w.WriteHeader(tt.args.statusCode)
				w.Write([]byte("some text"))
			}
			req, _ := http.NewRequest("GET", "/url", nil)
			handler(w, req)

			parser := HTTPResponseWithErrorParserFactory(tt.args.cfg)
			resp, err := parser(ctx, w.Result())
			if err != nil {
				t.Error("unexpected error:", err.Error())
			}
			if resp.Metadata.IsRequired != tt.wantMetaData.IsRequired {
				t.Error("unexpected result")
			}
			if resp.Metadata.StatusCode != tt.wantStatusCode {
				t.Error("unexpected status code")
			}
			if resp.IsComplete != tt.wantIsComplete {
				t.Error("unexpected status code")
			}
		})
	}
}
