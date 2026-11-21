package testcontainer

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIssueCertificates pins what issueCertificates hands a test server and
// its clients: a server certificate the authority vouches for under every
// host it was issued for, a host name and an IP address alike, and under no
// other; a client certificate the authority vouches for as a client's; and a
// key that matches each of the two.
func TestIssueCertificates(t *testing.T) {
	issued, err := issueCertificates("db.sample.test", "127.0.0.1")
	require.NoError(t, err)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(issued.authority))

	server := parseCertificate(t, issued.serverCert)
	for _, host := range []string{"db.sample.test", "127.0.0.1"} {
		_, err = server.Verify(x509.VerifyOptions{Roots: roots, DNSName: host, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
		require.NoError(t, err, host)
	}
	_, err = server.Verify(x509.VerifyOptions{Roots: roots, DNSName: "other.sample.test", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	require.Error(t, err, "the server certificate names the hosts it was issued for alone")

	client := parseCertificate(t, issued.clientCert)
	_, err = client.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	require.NoError(t, err)

	_, err = tls.X509KeyPair(issued.serverCert, issued.serverKey)
	require.NoError(t, err)
	_, err = tls.X509KeyPair(issued.clientCert, issued.clientKey)
	require.NoError(t, err)
}

// parseCertificate reads the one certificate certPEM holds.
func parseCertificate(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}
