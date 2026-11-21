package testcontainer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"github.com/cockroachdb/errors"
)

// certificates is the TLS material a test server runs with and a test dials
// it with, all PEM: the certificate of an authority made for the server, the
// certificate and key that authority signed for the server, and the
// certificate and key it signed for a client.
type certificates struct {
	authority  []byte
	serverCert []byte
	serverKey  []byte
	clientCert []byte
	clientKey  []byte
}

// certificateLifetime is how long the certificates of a test server hold: a
// server of the tests lives as long as the test binary that started it.
const certificateLifetime = 24 * time.Hour

// issueCertificates makes an authority and has it sign a certificate for a
// server reached at hosts, host names and IP addresses alike, and one for a
// client. The authority's key is dropped once the two are signed: nothing
// else is ever issued under it.
func issueCertificates(hosts ...string) (certificates, error) {
	notBefore := time.Now().Add(-time.Hour)
	notAfter := notBefore.Add(certificateLifetime)

	authorityKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return certificates{}, errors.Wrap(err, "failed to generate the authority key")
	}
	authority := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "gst test authority"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	authorityDER, err := x509.CreateCertificate(rand.Reader, authority, authority, &authorityKey.PublicKey, authorityKey)
	if err != nil {
		return certificates{}, errors.Wrap(err, "failed to sign the authority certificate")
	}

	server := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "gst test server"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			server.IPAddresses = append(server.IPAddresses, ip)
			continue
		}
		server.DNSNames = append(server.DNSNames, host)
	}
	client := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "gst test client"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	issued := certificates{authority: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: authorityDER})}
	if issued.serverCert, issued.serverKey, err = signCertificate(server, authority, authorityKey); err != nil {
		return certificates{}, errors.Wrap(err, "failed to issue the server certificate")
	}
	if issued.clientCert, issued.clientKey, err = signCertificate(client, authority, authorityKey); err != nil {
		return certificates{}, errors.Wrap(err, "failed to issue the client certificate")
	}
	return issued, nil
}

// signCertificate generates a key for template and has authority sign the
// certificate, returning the certificate and the key in PEM.
func signCertificate(template, authority *x509.Certificate, authorityKey *ecdsa.PrivateKey) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.CreateCertificate(rand.Reader, template, authority, &key.PublicKey, authorityKey)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}
