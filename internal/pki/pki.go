// Package pki provides certificate generation utilities for bootstrapping
// mTLS between the devopsctl controller and agent. It is intended for
// development and homelab use only. Production deployments should use
// certificates from an external CA.
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// InitOptions controls what Init generates.
type InitOptions struct {
	// OutputDir is the directory where certificate/key files are written.
	OutputDir string
	// ValidFor is how long generated certificates are valid. Default: 10 years.
	ValidFor time.Duration
	// CACommonName is the CN of the self-signed CA. Default: "devopsctl CA".
	CACommonName string
	// ControllerCommonName is the CN of the controller certificate. Default: "devopsctl controller".
	ControllerCommonName string
	// AgentCommonName is the CN of the agent certificate. Default: "devopsctl agent".
	AgentCommonName string
}

// CertBundle holds the paths to all generated files.
type CertBundle struct {
	CACert         string
	CAKey          string
	ControllerCert string
	ControllerKey  string
	AgentCert      string
	AgentKey       string
}

// Init generates a self-signed CA and two leaf certificates (controller and agent)
// signed by that CA. All files are written to opts.OutputDir with 0600 permissions
// for keys and 0644 for certificates.
func Init(opts InitOptions) (*CertBundle, error) {
	if opts.OutputDir == "" {
		return nil, fmt.Errorf("output directory must be specified")
	}
	if opts.ValidFor == 0 {
		opts.ValidFor = 10 * 365 * 24 * time.Hour
	}
	if opts.CACommonName == "" {
		opts.CACommonName = "devopsctl CA"
	}
	if opts.ControllerCommonName == "" {
		opts.ControllerCommonName = "devopsctl controller"
	}
	if opts.AgentCommonName == "" {
		opts.AgentCommonName = "devopsctl agent"
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory %q: %w", opts.OutputDir, err)
	}

	// 1. Generate CA key and self-signed certificate.
	caKey, caCert, caCertDER, err := generateCA(opts.CACommonName, opts.ValidFor)
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	// 2. Generate controller key and certificate signed by the CA.
	controllerKey, controllerCertDER, err := generateLeaf(opts.ControllerCommonName, opts.ValidFor, caKey, caCert)
	if err != nil {
		return nil, fmt.Errorf("generate controller cert: %w", err)
	}

	// 3. Generate agent key and certificate signed by the CA.
	agentKey, agentCertDER, err := generateLeaf(opts.AgentCommonName, opts.ValidFor, caKey, caCert)
	if err != nil {
		return nil, fmt.Errorf("generate agent cert: %w", err)
	}

	// 4. Write all files.
	bundle := &CertBundle{
		CACert:         filepath.Join(opts.OutputDir, "ca.crt"),
		CAKey:          filepath.Join(opts.OutputDir, "ca.key"),
		ControllerCert: filepath.Join(opts.OutputDir, "controller.crt"),
		ControllerKey:  filepath.Join(opts.OutputDir, "controller.key"),
		AgentCert:      filepath.Join(opts.OutputDir, "agent.crt"),
		AgentKey:       filepath.Join(opts.OutputDir, "agent.key"),
	}

	if err := writeCert(bundle.CACert, caCertDER); err != nil {
		return nil, err
	}
	if err := writeKey(bundle.CAKey, caKey); err != nil {
		return nil, err
	}
	if err := writeCert(bundle.ControllerCert, controllerCertDER); err != nil {
		return nil, err
	}
	if err := writeKey(bundle.ControllerKey, controllerKey); err != nil {
		return nil, err
	}
	if err := writeCert(bundle.AgentCert, agentCertDER); err != nil {
		return nil, err
	}
	if err := writeKey(bundle.AgentKey, agentKey); err != nil {
		return nil, err
	}

	return bundle, nil
}

// generateCA creates a new ECDSA P-256 key and a self-signed CA certificate.
func generateCA(cn string, validFor time.Duration) (*ecdsa.PrivateKey, *x509.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"devopsctl"},
		},
		NotBefore:             time.Now().Add(-time.Minute), // small backdate for clock skew
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, err
	}

	return key, cert, der, nil
}

// generateLeaf creates a new ECDSA P-256 key and a certificate signed by the given CA.
func generateLeaf(cn string, validFor time.Duration, caKey *ecdsa.PrivateKey, caCert *x509.Certificate) (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"devopsctl"},
		},
		NotBefore: time.Now().Add(-time.Minute),
		NotAfter:  time.Now().Add(validFor),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	return key, der, nil
}

func randomSerial() (*big.Int, error) {
	max := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}
	return n, nil
}

func writeCert(path string, der []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writeKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}
