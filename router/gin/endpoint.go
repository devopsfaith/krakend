package gin

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/core"
	"github.com/devopsfaith/krakend/proxy"
	"github.com/devopsfaith/krakend/router"
)

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
	requestGenerator := NewRequestByConfiguration(configuration)
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
			}

			for k, v := range response.Metadata.Headers {
				c.Header(k, v[0])
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
		headers["X-Forwarded-For"] = []string{c.ClientIP()}
		headers["User-Agent"] = router.UserAgentHeaderValue

		for _, k := range headersToSend {
			if h, ok := c.Request.Header[k]; ok {
				headers[k] = h
			}
		}

		query := make(map[string][]string, len(queryString))
		queryValues := c.Request.URL.Query()
		for i := range queryString {
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

// NewRequestByConfiguration gets a request from the current gin context, endpoint configuration and the received query string
func NewRequestByConfiguration(configuration *config.EndpointConfig) func(*gin.Context, []string) *proxy.Request {
	headersToSend := configuration.HeadersToPass
	if len(headersToSend) == 0 && !configuration.PassAllHeaders {
		headersToSend = router.HeadersToSend
	}

	return func(c *gin.Context, queryString []string) *proxy.Request {
		params := make(map[string]string, len(c.Params))
		for _, param := range c.Params {
			params[strings.Title(param.Key)] = param.Value
		}

		headers := make(map[string][]string, 2+len(headersToSend))
		if configuration.PassAllHeaders {
			headers = c.Request.Header
		} else {
			headers["X-Forwarded-For"] = []string{c.ClientIP()}
			headers["User-Agent"] = router.UserAgentHeaderValue

			for _, k := range headersToSend {
				if h, ok := c.Request.Header[k]; ok {
					headers[k] = h
				}
			}

		}

		query := make(map[string][]string, len(queryString))
		if configuration.PassAllQueryString {
			query = c.Request.URL.Query()
		} else {
			queryValues := c.Request.URL.Query()
			for i := range queryString {
				if v, ok := queryValues[queryString[i]]; ok && len(v) > 0 {
					query[queryString[i]] = v
				}
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
