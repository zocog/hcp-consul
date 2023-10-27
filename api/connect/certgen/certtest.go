package certgen

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/mitchellh/go-testing-interface"

	"github.com/hashicorp/consul/api"
)

// TestClusterID is the Consul cluster ID for testing.
//
// NOTE: this is duplicated in the api package as testClusterID
const TestClusterID = "11111111-2222-3333-4444-555555555555"
const TestTrustDomain = TestClusterID + ".consul"

// testCACounter is just an atomically incremented counter for creating
// unique names for the CA certs.
var testCACounter uint64

// TestCAWithKeyType is similar to TestCA, except that it
// takes two additional arguments to override the default private key type and size.
func TestCAWithKeyType(t testing.T, xc *api.CARoot, keyType string, keyBits int) *api.CARoot {
	return testCA(t, xc, keyType, keyBits, 0)
}

func testCA(t testing.T, xc *api.CARoot, keyType string, keyBits int, ttl time.Duration) *api.CARoot {
	var result api.CARoot
	result.Active = true
	result.Name = fmt.Sprintf("Test CA %d", atomic.AddUint64(&testCACounter, 1))

	// Create the private key we'll use for this CA cert.
	signer, keyPEM := testPrivateKey(t, keyType, keyBits)
	result.SigningKey = keyPEM
	result.SigningKeyID = EncodeSigningKeyID(testKeyID(t, signer.Public()))

	// The serial number for the cert
	sn, err := testSerialNumber()
	if err != nil {
		t.Fatalf("error generating serial number: %s", err)
	}

	// The URI (SPIFFE compatible) for the cert
	id := &SpiffeIDSigning{ClusterID: TestClusterID, Domain: "consul"}

	// Create the CA cert
	now := time.Now()
	before := now
	after := now
	if ttl != 0 {
		after = after.Add(ttl)
	} else {
		after = after.AddDate(10, 0, 0)
	}
	template := x509.Certificate{
		SerialNumber:          sn,
		Subject:               pkix.Name{CommonName: result.Name},
		URIs:                  []*url.URL{id.URI()},
		BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign |
			x509.KeyUsageCRLSign |
			x509.KeyUsageDigitalSignature,
		IsCA:           true,
		NotAfter:       after,
		NotBefore:      before,
		AuthorityKeyId: testKeyID(t, signer.Public()),
		SubjectKeyId:   testKeyID(t, signer.Public()),
	}

	bs, err := x509.CreateCertificate(
		rand.Reader, &template, &template, signer.Public(), signer)
	if err != nil {
		t.Fatalf("error generating CA certificate: %s", err)
	}

	var buf bytes.Buffer
	err = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: bs})
	if err != nil {
		t.Fatalf("error encoding private key: %s", err)
	}
	result.RootCert = buf.String()
	result.ID = CalculateCertFingerprint(bs)
	result.SerialNumber = uint64(sn.Int64())
	result.NotBefore = template.NotBefore.UTC()
	result.NotAfter = template.NotAfter.UTC()
	result.PrivateKeyType = keyType
	result.PrivateKeyBits = keyBits
	result.IntermediateCerts = []string{}

	// If there is a prior CA to cross-sign with, then we need to create that
	// and set it as the signing cert.
	if xc != nil {
		xccert, err := ParseCert(xc.RootCert)
		if err != nil {
			t.Fatalf("error parsing CA cert: %s", err)
		}
		xcsigner, err := ParseSigner(xc.SigningKey)
		if err != nil {
			t.Fatalf("error parsing signing key: %s", err)
		}

		// Set the authority key to be the previous one.
		// NOTE(mitchellh): From Paul Banks:  if we have to cross-sign a cert
		// that came from outside (e.g. vault) we can't rely on them using the
		// same KeyID hashing algo we do so we'd need to actually copy this
		// from the xc cert's subjectKeyIdentifier extension.
		template.AuthorityKeyId = testKeyID(t, xcsigner.Public())

		// Create the new certificate where the parent is the previous
		// CA, the public key is the new public key, and the signing private
		// key is the old private key.
		bs, err := x509.CreateCertificate(
			rand.Reader, &template, xccert, signer.Public(), xcsigner)
		if err != nil {
			t.Fatalf("error generating CA certificate: %s", err)
		}

		var buf bytes.Buffer
		err = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: bs})
		if err != nil {
			t.Fatalf("error encoding private key: %s", err)
		}
		result.SigningCert = buf.String()
	}

	return &result
}

// testPrivateKey creates an ECDSA based private key. Both a crypto.Signer and
// the key in PEM form are returned.
//
// NOTE(banks): this was memoized to save entropy during tests but it turns out
// crypto/rand will never block and always reads from /dev/urandom on unix OSes
// which does not consume entropy.
//
// If we find by profiling it's taking a lot of cycles we could optimize/cache
// again but we at least need to use different keys for each distinct CA (when
// multiple CAs are generated at once e.g. to test cross-signing) and a
// different one again for the leafs otherwise we risk tests that have false
// positives since signatures from different logical cert's keys are
// indistinguishable, but worse we build validation chains using AuthorityKeyID
// which will be the same for multiple CAs/Leafs. Also note that our UUID
// generator also reads from crypto rand and is called far more often during
// tests than this will be.
func testPrivateKey(t testing.T, keyType string, keyBits int) (crypto.Signer, string) {
	pk, pkPEM, err := GeneratePrivateKeyWithConfig(keyType, keyBits)
	if err != nil {
		t.Fatalf("error generating private key: %s", err)
	}

	return pk, pkPEM
}

// testSerialNumber generates a serial number suitable for a certificate. For
// testing, this just sets it to a random number, but one that can fit in a
// uint64 since we use that in our datastructures and assume cert serials will
// fit in that for now.
func testSerialNumber() (*big.Int, error) {
	return rand.Int(rand.Reader, (&big.Int{}).Exp(big.NewInt(2), big.NewInt(63), nil))
}

// testKeyID returns a KeyID from the given public key. This just calls
// KeyId but handles errors for tests.
func testKeyID(t testing.T, raw interface{}) []byte {
	result, err := KeyId(raw)
	if err != nil {
		t.Fatalf("KeyId error: %s", err)
	}

	return result
}
