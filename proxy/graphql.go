package proxy

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/url"
	"strconv"
	"strings"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/transport/http/client/graphql"
)

// NewGraphQLMiddleware returns a middleware with or without the GraphQL
// proxy wrapping the next element (depending on the configuration).
// It supports both queries and mutations.
// For queries, it completes the variables object using the request params.
// For mutations, it overides the defined variables with the request body.
// The resulting request will have a proper graphql body with the query and the
// variables
func NewGraphQLMiddleware(remote *config.Backend) Middleware {
	opt, err := graphql.GetOptions(remote.ExtraConfig)
	if err != nil {
		return EmptyMiddleware
	}

	extractor := graphql.New(*opt)
	var generateBodyFn func(*Request) ([]byte, error)
	var generateQueryFn func(*Request) (url.Values, error)

	switch opt.Type {
	case graphql.OperationMutation:
		generateBodyFn = func(req *Request) ([]byte, error) {
			if req.Body == nil {
				return extractor.BodyFromBody(strings.NewReader(""))
			}
			defer req.Body.Close()
			return extractor.BodyFromBody(req.Body)
		}
		generateQueryFn = func(req *Request) (url.Values, error) {
			if req.Body == nil {
				return extractor.QueryFromBody(strings.NewReader(""))
			}
			defer req.Body.Close()
			return extractor.QueryFromBody(req.Body)
		}

	case graphql.OperationQuery:
		generateBodyFn = func(req *Request) ([]byte, error) {
			return extractor.BodyFromParams(req.Params)
		}
		generateQueryFn = func(req *Request) (url.Values, error) {
			return extractor.QueryFromParams(req.Params)
		}

	default:
		return EmptyMiddleware
	}

	return func(next ...Proxy) Proxy {
		if len(next) > 1 {
			panic(ErrTooManyProxies)
		}

		if opt.Method == graphql.MethodGet {
			return func(ctx context.Context, req *Request) (*Response, error) {
				q, err := generateQueryFn(req)
				if err != nil {
					return nil, err
				}

				req.Body = ioutil.NopCloser(bytes.NewReader([]byte{}))
				req.Method = string(opt.Method)
				req.Headers["Content-Length"] = []string{"0"}
				if req.Query != nil {
					for k, vs := range q {
						for _, v := range vs {
							req.Query.Add(k, v)
						}
					}
				} else {
					req.Query = q
				}

				return next[0](ctx, req)
			}
		}

		return func(ctx context.Context, req *Request) (*Response, error) {
			b, err := generateBodyFn(req)
			if err != nil {
				return nil, err
			}

			req.Body = ioutil.NopCloser(bytes.NewReader(b))
			req.Method = string(opt.Method)
			req.Headers["Content-Length"] = []string{strconv.Itoa(len(string(b)))}

			return next[0](ctx, req)
		}
	}
}
