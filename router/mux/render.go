package mux

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/encoding"
	"github.com/devopsfaith/krakend/proxy"
)

// Render defines the signature of the functions to be use for the final response
// encoding and rendering
type Render func(http.ResponseWriter, *proxy.Response)

// NEGOTIATE defines the value of the OutputEncoding for the negotiated render
const NEGOTIATE = "negotiate"

var (
	mutex          = &sync.RWMutex{}
	renderRegister = map[string]Render{
		encoding.STRING: stringRender,
		encoding.JSON:   jsonRender,
		encoding.NOOP:   noopRender,
	}
)

// RegisterRender allows clients to register their custom renders
func RegisterRender(name string, r Render) {
	mutex.Lock()
	renderRegister[name] = r
	mutex.Unlock()
}

func getRender(cfg *config.EndpointConfig) Render {
	fallback := jsonRender
	if len(cfg.Backend) == 1 {
		fallback = getWithFallback(cfg.Backend[0].Encoding, fallback)
	}

	if cfg.OutputEncoding == "" {
		return fallback
	}

	return getWithFallback(cfg.OutputEncoding, fallback)
}

func getWithFallback(key string, fallback Render) Render {
	mutex.RLock()
	r, ok := renderRegister[key]
	mutex.RUnlock()
	if !ok {
		return fallback
	}
	return r
}

var emptyResponse = []byte("{}")

func jsonRender(w http.ResponseWriter, response *proxy.Response) {
	w.Header().Set("Content-Type", "application/json")
	if response == nil {
		w.Write(emptyResponse)
		return
	}

	js, err := json.Marshal(response.Data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if response.Metadata.IsRequired && response.Metadata.StatusCode > 0 {
		w.WriteHeader(response.Metadata.StatusCode)
	}
	w.Write(js)
}

func stringRender(w http.ResponseWriter, response *proxy.Response) {
	w.Header().Set("Content-Type", "text/plain")
	if response == nil {
		w.Write([]byte{})
		return
	}
	d, ok := response.Data["content"]
	if !ok {
		w.Write([]byte{})
		return
	}
	msg, ok := d.(string)
	if !ok {
		w.Write([]byte{})
		return
	}
	if response.Metadata.IsRequired && response.Metadata.StatusCode > 0 {
		w.WriteHeader(response.Metadata.StatusCode)
	}
	w.Write([]byte(msg))
}

func noopRender(w http.ResponseWriter, response *proxy.Response) {
	if response == nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	for k, vs := range response.Metadata.Headers {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(response.Metadata.StatusCode)

	io.Copy(w, response.Io)
}
