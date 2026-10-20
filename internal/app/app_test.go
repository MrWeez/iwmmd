package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRedirectToBase(t *testing.T) {
	base, err := url.Parse("https://domains.example.com")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://domains.example.com/domains/example.com?x=1", nil)
	rec := httptest.NewRecorder()
	redirectToBase(base).ServeHTTP(rec, req)
	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://domains.example.com/domains/example.com?x=1" {
		t.Fatalf("Location = %q", got)
	}
}

func TestCertReloader(t *testing.T) {
	host := "domains.example.com"
	certPath, keyPath := writeTestCert(t, host)
	reloader, err := newCertReloader(certPath, keyPath, host)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := reloader.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Leaf == nil {
		t.Fatal("leaf certificate was not parsed")
	}
	if err := cert.Leaf.VerifyHostname(host); err != nil {
		t.Fatal(err)
	}
	if err := reloader.Reload(); err != nil {
		t.Fatal(err)
	}
}

func TestCertReloaderRejectsWrongHost(t *testing.T) {
	certPath, keyPath := writeTestCert(t, "other.example.com")
	if _, err := newCertReloader(certPath, keyPath, "domains.example.com"); err == nil {
		t.Fatal("expected hostname mismatch")
	}
}

func writeTestCert(t *testing.T, host string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
