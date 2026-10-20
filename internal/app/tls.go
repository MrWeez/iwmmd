package app

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
)

type certReloader struct {
	certFile string
	keyFile  string
	host     string

	mu   sync.RWMutex
	cert *tls.Certificate
}

func newCertReloader(certFile, keyFile, host string) (*certReloader, error) {
	r := &certReloader{certFile: certFile, keyFile: keyFile, host: host}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *certReloader) Reload() error {
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("load TLS certificate: %w", err)
	}
	if cert.Leaf == nil {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("parse TLS certificate: %w", err)
		}
		cert.Leaf = leaf
	}
	if r.host != "" {
		if err := cert.Leaf.VerifyHostname(r.host); err != nil {
			return fmt.Errorf("TLS certificate does not match %s: %w", r.host, err)
		}
	}
	r.mu.Lock()
	r.cert = &cert
	r.mu.Unlock()
	return nil
}

func (r *certReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cert == nil {
		return nil, fmt.Errorf("TLS certificate is not loaded")
	}
	return r.cert, nil
}
