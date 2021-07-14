package graphql

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/devopsfaith/krakend/config"
)

// Namespace is the key for the backend's extra config
const Namespace = "github.com/devopsfaith/krakend/transport/http/client/graphql"

// OperationType contains all the operations allowed by graphql
type OperationType string

// OperationMethod details the method to be used with the request
type OperationMethod string

const (
	// OperationMutation marks an operation as a mutation
	OperationMutation OperationType = "mutation"
	// OperationQuery marks an operation as a query
	OperationQuery OperationType = "query"

	MethodPost OperationMethod = http.MethodPost
	MethodGet  OperationMethod = http.MethodGet
)

// GraphQLRequest represents the graphql request body
type GraphQLRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

// Options defines a GraphQLRequest with a type, so the middlewares know what to do
type Options struct {
	GraphQLRequest
	Type   OperationType   `json:"type"`
	Method OperationMethod `json:"method"`
}

var errNoConfigFound = errors.New("grapghql: no configuration found")

// GetOptions extracts the Options config from the backend's extra config
func GetOptions(cfg config.ExtraConfig) (*Options, error) {
	tmp, ok := cfg[Namespace]
	if !ok {
		return nil, errNoConfigFound
	}

	b, err := json.Marshal(tmp)
	if err != nil {
		return nil, err
	}

	var opt Options
	if err := json.Unmarshal(b, &opt); err != nil {
		return nil, err
	}

	opt.Method = OperationMethod(strings.ToUpper(string(opt.Method)))
	opt.Type = OperationType(strings.ToLower(string(opt.Type)))

	if opt.Method != MethodGet && opt.Method != MethodPost {
		opt.Method = MethodPost
	}

	return &opt, nil
}

// New resturns a new Extractor, ready to be use on a middleware
func New(opt Options) Extractor {
	replacements := [][2]string{}
	for k, v := range opt.Variables {
		val, ok := v.(string)
		if !ok {
			continue
		}
		if val[0] == '{' && val[len(val)-1] == '}' {
			replacements = append(replacements, [2]string{k, strings.Title(val[1:2]) + val[2:len(val)-1]})
		}
	}

	if len(replacements) == 0 {
		b, _ := json.Marshal(opt.GraphQLRequest)

		return Extractor{
			cfg: opt,
			paramExtractor: func(map[string]string) (*GraphQLRequest, error) {
				return &opt.GraphQLRequest, nil
			},
			newBody: func(_ map[string]string) ([]byte, error) {
				return b, nil
			},
		}
	}

	paramExtractor := func(params map[string]string) (*GraphQLRequest, error) {
		val := GraphQLRequest{
			Query:         opt.Query,
			OperationName: opt.OperationName,
			Variables:     map[string]interface{}{},
		}
		for k, v := range opt.Variables {
			val.Variables[k] = v
		}
		for _, vs := range replacements {
			val.Variables[vs[0]] = params[vs[1]]
		}
		return &val, nil
	}
	return Extractor{
		cfg:            opt,
		paramExtractor: paramExtractor,
		newBody: func(params map[string]string) ([]byte, error) {
			val, err := paramExtractor(params)
			if err != nil {
				return []byte{}, err
			}
			return json.Marshal(val)
		},
	}
}

// Extractor exposes two extractor factories: one for the params (query) and one
// for the request body (mutator)
type Extractor struct {
	cfg            Options
	paramExtractor func(map[string]string) (*GraphQLRequest, error)
	newBody        func(map[string]string) ([]byte, error)
}

// QueryFromBody returns a url.Values containing the graphql request with the given query and the default variables
// overiden by the request body
func (e Extractor) QueryFromBody(r io.Reader) (url.Values, error) {
	gr, err := e.fromBody(r)
	if err != nil {
		return nil, err
	}
	vars := url.Values{}

	vars.Add("query", gr.Query)
	if gr.OperationName != "" {
		vars.Add("operationName", gr.OperationName)
	}
	if len(gr.Variables) != 0 {
		encodedVars, _ := json.Marshal(gr.Variables)
		vars.Add("variables", string(encodedVars))
	}

	return vars, nil
}

// BodyFromBody returns a request body containing the graphql request with the given query and the default variables
// overiden by the request body
func (e Extractor) BodyFromBody(r io.Reader) ([]byte, error) {
	v, err := e.fromBody(r)
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(v)
}

func (e Extractor) fromBody(r io.Reader) (*GraphQLRequest, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	vars := map[string]interface{}{}

	if err := json.Unmarshal(b, &vars); err != nil {
		return nil, err
	}

	for k, v := range e.cfg.Variables {
		if _, ok := vars[k]; ok {
			continue
		}
		vars[k] = v
	}

	return &GraphQLRequest{
		Query:         e.cfg.Query,
		OperationName: e.cfg.OperationName,
		Variables:     vars,
	}, nil
}

// QueryFromParams returns a url.Values containing the grapql request generated for the given query and the default
// variables overiden by the request params
func (e Extractor) QueryFromParams(params map[string]string) (url.Values, error) {
	gr, err := e.paramExtractor(params)
	if err != nil {
		return nil, err
	}
	vars := url.Values{}

	vars.Add("query", gr.Query)
	if gr.OperationName != "" {
		vars.Add("operationName", gr.OperationName)
	}
	if len(gr.Variables) != 0 {
		encodedVars, _ := json.Marshal(gr.Variables)
		vars.Add("variables", string(encodedVars))
	}

	return vars, nil
}

// BodyFromParams returns a request body containing the grapql request generated for the given query and the default
// variables overiden by the request params
func (e Extractor) BodyFromParams(params map[string]string) ([]byte, error) {
	return e.newBody(params)
}
