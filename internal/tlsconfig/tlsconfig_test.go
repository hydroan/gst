package tlsconfig_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/tlsconfig"
	"github.com/stretchr/testify/require"
)

// TestBuildRefusesHalfOfAKeyPair pins that a client certificate is
// configured by both of its files or by neither: one given without the other
// is refused naming the file that is missing, rather than dialed with no
// certificate, which the far end answers with a handshake failure that says
// nothing of the configuration.
func TestBuildRefusesHalfOfAKeyPair(t *testing.T) {
	_, err := tlsconfig.Build("client.crt", "", "", false)
	require.EqualError(t, err, "cert_file is set without key_file: a client certificate takes both")

	_, err = tlsconfig.Build("", "client.key", "", false)
	require.EqualError(t, err, "key_file is set without cert_file: a client certificate takes both")
}

// TestBuildCarriesWhatIsConfigured pins the configuration a client
// dials with: no certificate and the system roots when no file is given, the
// key pair when both of its files are, the bundle of caFile as the roots,
// and insecureSkipVerify as given.
func TestBuildCarriesWhatIsConfigured(t *testing.T) {
	certFile, keyFile := writeKeyPair(t)

	bare, err := tlsconfig.Build("", "", "", true)
	require.NoError(t, err)
	require.Empty(t, bare.Certificates)
	require.Nil(t, bare.RootCAs)
	require.True(t, bare.InsecureSkipVerify)

	full, err := tlsconfig.Build(certFile, keyFile, certFile, false)
	require.NoError(t, err)
	require.Len(t, full.Certificates, 1)
	require.NotNil(t, full.RootCAs)
	require.False(t, full.InsecureSkipVerify)

	_, err = tlsconfig.Build("", "", keyFile, false)
	require.EqualError(t, err, "failed to append ca certs", "a bundle holding no certificate is refused")
}

// writeKeyPair writes a self-signed certificate and its key as PEM files
// under the test's temporary directory and returns their paths.
func writeKeyPair(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sample.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile
}
