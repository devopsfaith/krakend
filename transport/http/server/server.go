package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/core"
)

// ToHTTPError translates an error into a HTTP status code
type ToHTTPError func(error) int

// DefaultToHTTPError is a ToHTTPError transalator that always returns an
// internal server error
func DefaultToHTTPError(_ error) int {
	return http.StatusInternalServerError
}

const (
	// HeaderCompleteResponseValue is the value of the CompleteResponseHeader
	// if the response is complete
	HeaderCompleteResponseValue = "true"
	// HeaderIncompleteResponseValue is the value of the CompleteResponseHeader
	// if the response is not complete
	HeaderIncompleteResponseValue = "false"
)

var (
	// CompleteResponseHeaderName is the header to flag incomplete responses to the client
	CompleteResponseHeaderName = "X-KrakenD-Completed"
	// HeadersToSend are the headers to pass from the router request to the proxy
	HeadersToSend = []string{"Content-Type"}
	// UserAgentHeaderValue is the value of the User-Agent header to add to the proxy request
	UserAgentHeaderValue = []string{core.KrakendUserAgent}

	// ErrInternalError is the error returned by the router when something went wrong
	ErrInternalError = errors.New("internal server error")
	// ErrPrivateKey is the error returned by the router when the private key is not defined
	ErrPrivateKey = errors.New("private key not defined")
	// ErrPublicKey is the error returned by the router when the public key is not defined
	ErrPublicKey = errors.New("public key not defined")
)

// InitHTTPDefaultTransport ensures the default HTTP transport is configured just once per execution
func InitHTTPDefaultTransport(cfg config.ServiceConfig) {
	onceTransportConfig.Do(func() {
		http.DefaultTransport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:       cfg.DialerTimeout,
				KeepAlive:     cfg.DialerKeepAlive,
				FallbackDelay: cfg.DialerFallbackDelay,
				DualStack:     true,
			}).DialContext,
			DisableCompression:    cfg.DisableCompression,
			DisableKeepAlives:     cfg.DisableKeepAlives,
			MaxIdleConns:          cfg.MaxIdleConns,
			MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
			IdleConnTimeout:       cfg.IdleConnTimeout,
			ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
			ExpectContinueTimeout: cfg.ExpectContinueTimeout,
			TLSHandshakeTimeout:   10 * time.Second,
		}
	})
}

// RunServer runs a http.Server with the given handler and configuration.
// It configures the TLS layer if required by the received configuration.
func RunServer(ctx context.Context, cfg config.ServiceConfig, handler http.Handler) error {
	done := make(chan error)
	s := NewServer(cfg, handler)

	if s.TLSConfig == nil {
		go func() {
			done <- s.ListenAndServe()
		}()
	} else {
		if cfg.TLS.PublicKey == "" {
			return ErrPublicKey
		}
		if cfg.TLS.PrivateKey == "" {
			return ErrPrivateKey
		}
		go func() {
			done <- s.ListenAndServeTLS(cfg.TLS.PublicKey, cfg.TLS.PrivateKey)
		}()
	}

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

// NewServer returns a http.Server ready to serve the injected handler
func NewServer(cfg config.ServiceConfig, handler http.Handler) *http.Server {
	srv := &http.Server{
		// Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		TLSConfig:         ParseTLSConfig(cfg.TLS),
	}

	if cfg.Address != "" {
		srv.Addr = cfg.Address
	} else {
		srv.Addr = fmt.Sprintf(":%d", cfg.Port)
	}

	return srv
}

// ParseTLSConfig creates a tls.Config from the TLS section of the service configuration
func ParseTLSConfig(cfg *config.TLS) *tls.Config {
	if cfg == nil {
		return nil
	}
	if cfg.IsDisabled {
		return nil
	}

	return &tls.Config{
		MinVersion:               parseTLSVersion(cfg.MinVersion),
		MaxVersion:               parseTLSVersion(cfg.MaxVersion),
		CurvePreferences:         parseCurveIDs(cfg),
		PreferServerCipherSuites: cfg.PreferServerCipherSuites,
		CipherSuites:             parseCipherSuites(cfg),
	}
}

func parseTLSVersion(key string) uint16 {
	if v, ok := versions[key]; ok {
		return v
	}
	return tls.VersionTLS12
}

func parseCurveIDs(cfg *config.TLS) []tls.CurveID {
	l := len(cfg.CurvePreferences)
	if l == 0 {
		return defaultCurves
	}

	curves := make([]tls.CurveID, len(cfg.CurvePreferences))
	for i := range curves {
		curves[i] = tls.CurveID(cfg.CurvePreferences[i])
	}
	return curves
}

func parseCipherSuites(cfg *config.TLS) []uint16 {
	l := len(cfg.CipherSuites)
	if l == 0 {
		return defaultCipherSuites
	}

	cs := make([]uint16, l)
	for i := range cs {
		cs[i] = uint16(cfg.CipherSuites[i])
	}
	return cs
}

var (
	onceTransportConfig sync.Once
	defaultCurves       = []tls.CurveID{
		tls.CurveP521,
		tls.CurveP384,
		tls.CurveP256,
	}
	defaultCipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}
	versions = map[string]uint16{
		"SSL3.0": tls.VersionSSL30,
		"TLS10":  tls.VersionTLS10,
		"TLS11":  tls.VersionTLS11,
		"TLS12":  tls.VersionTLS12,
	}
)
