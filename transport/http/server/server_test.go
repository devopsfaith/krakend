//go:generate go run $GOROOT/src/crypto/tls/generate_cert.go --rsa-bits 1024 --host 127.0.0.1,::1,localhost --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devopsfaith/krakend/config"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func TestRunServer_TLS(t *testing.T) {
	testKeysAreAvailable(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := newPort()

	done := make(chan error)
	go func() {
		done <- RunServer(
			ctx,
			config.ServiceConfig{
				Port: port,
				TLS: &config.TLS{
					PublicKey:  "cert.pem",
					PrivateKey: "key.pem",
				},
			},
			http.HandlerFunc(dummyHandler),
		)
	}()

	client, err := httpsClient("cert.pem")
	if err != nil {
		t.Error(err)
		return
	}

	<-time.After(100 * time.Millisecond)

	resp, err := client.Get(fmt.Sprintf("https://localhost:%d", port))
	if err != nil {
		t.Error(err)
		return
	}
	if resp.StatusCode != 200 {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
		return
	}
	cancel()

	if err = <-done; err != nil {
		t.Error(err)
	}
}

func TestRunServer_plain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := newPort()

	done := make(chan error)
	go func() {
		done <- RunServer(
			ctx,
			config.ServiceConfig{Port: port},
			http.HandlerFunc(dummyHandler),
		)
	}()

	<-time.After(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		t.Error(err)
		return
	}
	if resp.StatusCode != 200 {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
		return
	}
	cancel()

	if err = <-done; err != nil {
		t.Error(err)
	}
}

func TestRunServer_address(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := newPort()
	port2 := newPort()

	done := make(chan error)
	go func() {
		done <- RunServer(
			ctx,
			config.ServiceConfig{
				Port:    port,
				Address: fmt.Sprintf(":%d", port2),
			},
			http.HandlerFunc(dummyHandler),
		)
	}()

	<-time.After(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d", port2))
	if err != nil {
		t.Error(err)
		return
	}
	if resp.StatusCode != 200 {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
		return
	}

	_, err2 := http.Get(fmt.Sprintf("http://localhost:%d", port))
	if err2 == nil {
		t.Error("expected `connection refused` error")
		return
	} else {
		if !strings.Contains(err2.Error(), "connect: connection refused") {
			t.Errorf("expected `connection refused` error; got %v", err2)
		}
	}
	cancel()

	if err = <-done; err != nil {
		t.Error(err)
	}
}

func TestRunServer_disabledTLS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error)

	port := newPort()

	go func() {
		done <- RunServer(
			ctx,
			config.ServiceConfig{
				Port: port,
				TLS: &config.TLS{
					IsDisabled: true,
				}},
			http.HandlerFunc(dummyHandler),
		)
	}()

	<-time.After(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		t.Error(err)
		return
	}
	if resp.StatusCode != 200 {
		t.Errorf("unexpected status code: %d", resp.StatusCode)
		return
	}
	cancel()

	if err = <-done; err != nil {
		t.Error(err)
	}
}

func TestRunServer_err(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error)
	for _, tc := range []struct {
		cfg *config.TLS
		err error
	}{
		{
			cfg: &config.TLS{},
			err: ErrPublicKey,
		},
		{
			cfg: &config.TLS{
				PublicKey: "unknown",
			},
			err: ErrPrivateKey,
		},
	} {
		go func() {
			done <- RunServer(
				ctx,
				config.ServiceConfig{TLS: tc.cfg},
				http.HandlerFunc(dummyHandler),
			)
		}()
		if err := <-done; err != tc.err {
			t.Error(err)
		}
	}
}

func TestRunServer_errBadKeys(t *testing.T) {
	done := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- RunServer(
			ctx,
			config.ServiceConfig{TLS: &config.TLS{
				PublicKey:  "unknown",
				PrivateKey: "unknown",
			}},
			http.HandlerFunc(dummyHandler),
		)
	}()
	if err := <-done; err == nil || err.Error() != "open unknown: no such file or directory" {
		t.Error(err)
	}
}

func Test_parseTLSVersion(t *testing.T) {
	for _, tc := range []struct {
		in  string
		out uint16
	}{
		{in: "SSL3.0", out: tls.VersionSSL30},
		{in: "TLS10", out: tls.VersionTLS10},
		{in: "TLS11", out: tls.VersionTLS11},
		{in: "TLS12", out: tls.VersionTLS12},
		{in: "Unknown", out: tls.VersionTLS12},
	} {
		if res := parseTLSVersion(tc.in); res != tc.out {
			t.Errorf("input %s generated output %d. expected: %d", tc.in, res, tc.out)
		}
	}
}

func Test_parseCurveIDs(t *testing.T) {
	original := []uint16{1, 2, 3}
	cs := parseCurveIDs(&config.TLS{CurvePreferences: original})
	for k, v := range cs {
		if original[k] != uint16(v) {
			t.Errorf("unexpected curves %v. expected: %v", cs, original)
		}
	}
}

func Test_parseCipherSuites(t *testing.T) {
	original := []uint16{1, 2, 3}
	cs := parseCipherSuites(&config.TLS{CipherSuites: original})
	for k, v := range cs {
		if original[k] != uint16(v) {
			t.Errorf("unexpected ciphersuites %v. expected: %v", cs, original)
		}
	}
}

func dummyHandler(rw http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(rw, "Hello, %q", html.EscapeString(req.URL.Path))
}

func testKeysAreAvailable(t *testing.T) {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		log.Fatal(err)
	}

	for _, k := range []string{"cert.pem", "key.pem"} {
		var exists bool
		for _, file := range files {
			if file.Name() == k {
				exists = true
				break
			}
		}
		if !exists {
			t.Errorf("file %s not present", k)
		}
	}
}

func httpsClient(cert string) (*http.Client, error) {
	cer, err := ioutil.ReadFile(cert)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(cer)
	if !ok {
		return nil, errors.New("failed to parse root certificate")
	}
	tlsConf := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
		RootCAs: roots,
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConf}}, nil
}

// newPort returns random port numbers to avoid port collisions during the tests
func newPort() int {
	return 16666 + rand.Intn(40000)
}
