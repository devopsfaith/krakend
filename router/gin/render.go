package gin

import (
	"io"
	"net/http"
	"sync"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/encoding"
	"github.com/devopsfaith/krakend/proxy"
	"github.com/gin-gonic/gin"
)

// Render defines the signature of the functions to be use for the final response
// encoding and rendering
type Render func(*gin.Context, *proxy.Response)

// NEGOTIATE defines the value of the OutputEncoding for the negotiated render
const NEGOTIATE = "negotiate"

var (
	mutex          = &sync.RWMutex{}
	renderRegister = map[string]Render{
		NEGOTIATE:       negotiatedRender,
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

func negotiatedRender(c *gin.Context, response *proxy.Response) {
	switch c.NegotiateFormat(gin.MIMEJSON, gin.MIMEPlain, gin.MIMEXML) {
	case gin.MIMEXML:
		xmlRender(c, response)
	case gin.MIMEPlain:
		yamlRender(c, response)
	default:
		jsonRender(c, response)
	}
}

func stringRender(c *gin.Context, response *proxy.Response) {
	status := c.Writer.Status()

	if response == nil {
		c.String(status, "")
		return
	}
	d, ok := response.Data["content"]
	if !ok {
		c.String(status, "")
		return
	}
	msg, ok := d.(string)
	if !ok {
		c.String(status, "")
		return
	}
	c.String(status, msg)
}

func jsonRender(c *gin.Context, response *proxy.Response) {
	status := c.Writer.Status()
	if response != nil && response.Metadata.StatusCode != 0 {
		status = response.Metadata.StatusCode
	}
	if response == nil {
		c.JSON(status, emptyResponse)
		return
	}
	c.JSON(status, response.Data)
}

func xmlRender(c *gin.Context, response *proxy.Response) {
	status := c.Writer.Status()
	if response == nil {
		c.XML(status, nil)
		return
	}
	d, ok := response.Data["content"]
	if !ok {
		c.XML(status, nil)
		return
	}
	c.XML(status, d)
}

func yamlRender(c *gin.Context, response *proxy.Response) {
	status := c.Writer.Status()
	if response == nil {
		c.YAML(status, emptyResponse)
		return
	}
	c.YAML(status, response.Data)
}

func noopRender(c *gin.Context, response *proxy.Response) {
	if response == nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(response.Metadata.StatusCode)
	for k, vs := range response.Metadata.Headers {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	if response.Io == nil {
		return
	}
	io.Copy(c.Writer, response.Io)
}

var emptyResponse = gin.H{}
