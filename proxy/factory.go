// SPDX-License-Identifier: Apache-2.0
package proxy

import (
	"github.com/luraproject/lura/config"
	"github.com/luraproject/lura/logging"
	"github.com/luraproject/lura/sd"
)

// Factory creates proxies based on the received endpoint configuration.
//
// Both, factories and backend factories, create proxies but factories are designed as a stack makers
// because they are intended to generate the complete proxy stack for a given frontend endpoint
// the app would expose and they could wrap several proxies provided by a backend factory
type Factory interface {
	New(cfg *config.EndpointConfig) (Proxy, error)
}

// FactoryFunc type is an adapter to allow the use of ordinary functions as proxy factories.
// If f is a function with the appropriate signature, FactoryFunc(f) is a Factory that calls f.
type FactoryFunc func(*config.EndpointConfig) (Proxy, error)

// New implements the Factory interface
func (f FactoryFunc) New(cfg *config.EndpointConfig) (Proxy, error) { return f(cfg) }

// DefaultFactory returns a default http proxy factory with the injected logger
func DefaultFactory(logger logging.Logger) Factory {
	return NewDefaultFactory(httpProxy, logger)
}

// DefaultFactoryWithSubscriber returns a default proxy factory with the injected logger and subscriber factory
func DefaultFactoryWithSubscriber(logger logging.Logger, sF sd.SubscriberFactory) Factory {
	return NewDefaultFactoryWithSubscriber(httpProxy, logger, sF)
}

// NewDefaultFactory returns a default proxy factory with the injected proxy builder and logger
func NewDefaultFactory(backendFactory BackendFactory, logger logging.Logger) Factory {
	return NewDefaultFactoryWithSubscriber(backendFactory, logger, sd.GetSubscriber)
}

// NewDefaultFactoryWithSubscriber returns a default proxy factory with the injected proxy builder,
// logger and subscriber factory
func NewDefaultFactoryWithSubscriber(backendFactory BackendFactory, logger logging.Logger, sF sd.SubscriberFactory) Factory {
	return defaultFactory{backendFactory, logger, sF}
}

type defaultFactory struct {
	backendFactory    BackendFactory
	logger            logging.Logger
	subscriberFactory sd.SubscriberFactory
}

// New implements the Factory interface
func (pf defaultFactory) New(cfg *config.EndpointConfig) (p Proxy, err error) {
	switch len(cfg.Backend) {
	case 0:
		err = ErrNoBackends
	case 1:
		p, err = pf.newSingle(cfg)
	default:
		p, err = pf.newMulti(cfg)
	}
	if err != nil {
		return
	}

	p = NewPluginMiddleware(cfg)(p)
	p = NewStaticMiddleware(cfg)(p)
	return
}

func (pf defaultFactory) newMulti(cfg *config.EndpointConfig) (p Proxy, err error) {
	backendProxy := make([]Proxy, len(cfg.Backend))
	for i, backend := range cfg.Backend {
		backendProxy[i] = pf.newStack(backend)
	}
	p = NewMergeDataMiddleware(cfg)(backendProxy...)
	p = NewFlatmapMiddleware(cfg)(p)
	return
}

func (pf defaultFactory) newSingle(cfg *config.EndpointConfig) (Proxy, error) {
	return pf.newStack(cfg.Backend[0]), nil
}

func (pf defaultFactory) newStack(backend *config.Backend) (p Proxy) {
	p = pf.backendFactory(backend)
	p = NewBackendPluginMiddleware(backend)(p)
	p = NewGraphQLMiddleware(backend)(p)
	p = NewLoadBalancedMiddlewareWithSubscriber(pf.subscriberFactory(backend))(p)
	if backend.ConcurrentCalls > 1 {
		p = NewConcurrentMiddleware(backend)(p)
	}
	p = NewRequestBuilderMiddleware(backend)(p)
	return
}
