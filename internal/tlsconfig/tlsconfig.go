// Package tlsconfig builds the TLS configuration a client of the framework
// dials with, from the four settings every component configures it by: the
// certificate file, its key file, the CA file and whether to skip
// verification. The mysql dialect, the migration, the providers and the redis
// client read the same settings and share no package of their own, so the
// rule lives in one here.
package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/cockroachdb/errors"
)

// Build builds the TLS configuration a client dials with: the certificate
// and key pair when both files are given, the CA bundle when caFile is, and
// insecureSkipVerify as given. One file of the pair given without the other
// is a configuration error, refused naming the file that is missing by the
// cert_file and key_file every component configures the pair under: dialing
// on without the certificate would only fail the handshake at the far end,
// which says nothing of the configuration.
func Build(certFile, keyFile, caFile string, insecureSkipVerify bool) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec
	}

	switch {
	case certFile != "" && keyFile != "":
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load x509 key pair")
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	case certFile != "":
		return nil, errors.New("cert_file is set without key_file: a client certificate takes both")
	case keyFile != "":
		return nil, errors.New("key_file is set without cert_file: a client certificate takes both")
	}

	if len(caFile) > 0 {
		caData, err := os.ReadFile(caFile)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read ca file")
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caData) {
			return nil, errors.New("failed to append ca certs")
		}
		tlsConfig.RootCAs = caPool
	}

	return tlsConfig, nil
}
