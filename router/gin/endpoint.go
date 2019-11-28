package gin

import (
	"context"
	"fmt"
	"net/textproto"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/core"
	"github.com/devopsfaith/krakend/proxy"
	"github.com/devopsfaith/krakend/router"
)

const requestParamsAsterisk string = "*"

// HandlerFactory creates a handler function that adapts the gin router with the injected proxy
type HandlerFactory func(*config.EndpointConfig, proxy.Proxy) gin.HandlerFunc

// EndpointHandler implements the HandleFactory interface using the default ToHTTPError function
func EndpointHandler(configuration *config.EndpointConfig, proxy proxy.Proxy) gin.HandlerFunc {
	return CustomErrorEndpointHandler(configuration, proxy, router.DefaultToHTTPError)
}

// CustomErrorEndpointHandler implements the HandleFactory interface
func CustomErrorEndpointHandler(configuration *config.EndpointConfig, prxy proxy.Proxy, errF router.ToHTTPError) gin.HandlerFunc {
	cacheControlHeaderValue := fmt.Sprintf("public, max-age=%d", int(configuration.CacheTTL.Seconds()))
	isCacheEnabled := configuration.CacheTTL.Seconds() != 0
	requestGenerator := NewRequest(configuration.HeadersToPass)
	render := getRender(configuration)

	return func(c *gin.Context) {
		requestCtx, cancel := context.WithTimeout(c, configuration.Timeout)

		c.Header(core.KrakendHeaderName, core.KrakendHeaderValue)

		response, err := prxy(requestCtx, requestGenerator(c, configuration.QueryString))

		select {
		case <-requestCtx.Done():
			if err == nil {
				err = router.ErrInternalError
			}
		default:
		}

		complete := router.HeaderIncompleteResponseValue

		if response != nil && len(response.Data) > 0 {
			if response.IsComplete {
				complete = router.HeaderCompleteResponseValue
				if isCacheEnabled {
					c.Header("Cache-Control", cacheControlHeaderValue)
				}
			} else if response.Metadata.IsRequired {
				c.Status(response.Metadata.StatusCode)
			}

			for k, vs := range response.Metadata.Headers {
				for _, v := range vs {
					c.Writer.Header().Add(k, v)
				}
			}
		}

		c.Header(router.CompleteResponseHeaderName, complete)

		if err != nil {
			c.Error(err)

			if response == nil {
				if t, ok := err.(responseError); ok {
					c.Status(t.StatusCode())
				} else {
					c.Status(errF(err))
				}
				cancel()
				return
			}
		}

		render(c, response)
		cancel()
	}
}

// NewRequest gets a request from the current gin context and the received query string
func NewRequest(headersToSend []string) func(*gin.Context, []string) *proxy.Request {
	if len(headersToSend) == 0 {
		headersToSend = router.HeadersToSend
	}

	return func(c *gin.Context, queryString []string) *proxy.Request {
		params := make(map[string]string, len(c.Params))
		for _, param := range c.Params {
			params[strings.Title(param.Key)] = param.Value
		}

		headers := make(map[string][]string, 2+len(headersToSend))

		for _, k := range headersToSend {
			if k == requestParamsAsterisk {
				headers = c.Request.Header

				break
			}

			if h, ok := c.Request.Header[textproto.CanonicalMIMEHeaderKey(k)]; ok {
				headers[k] = h
			}
		}

		headers["X-Forwarded-For"] = []string{c.ClientIP()}
		// if User-Agent is not forwarded using headersToSend, we set
		// the KrakenD router User Agent value
		if _, ok := headers["User-Agent"]; !ok {
			headers["User-Agent"] = router.UserAgentHeaderValue
		} else {
			headers["X-Forwarded-Via"] = router.UserAgentHeaderValue
		}

		query := make(map[string][]string, len(queryString))
		queryValues := c.Request.URL.Query()
		for i := range queryString {
			if queryString[i] == requestParamsAsterisk {
				query = c.Request.URL.Query()

				break
			}

			if v, ok := queryValues[queryString[i]]; ok && len(v) > 0 {
				query[queryString[i]] = v
			}
		}

		return &proxy.Request{
			Method:  c.Request.Method,
			Query:   query,
			Body:    c.Request.Body,
			Params:  params,
			Headers: headers,
		}
	}
}

type responseError interface {
	error
	StatusCode() int
}
