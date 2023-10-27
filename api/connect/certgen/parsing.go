package certgen

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

// EncodeSigningKeyID encodes the given AuthorityKeyId or SubjectKeyId into a
// colon-hex encoded string suitable for using as a SigningKeyID value.
func EncodeSigningKeyID(keyID []byte) string { return HexString(keyID) }

// HexString returns a standard colon-separated hex value for the input
// byte slice. This should be used with cert serial numbers and so on.
func HexString(input []byte) string {
	return strings.Replace(fmt.Sprintf("% x", input), " ", ":", -1)
}

// KeyId returns a x509 KeyId from the given signing key. The key must be
// an *ecdsa.PublicKey currently, but may support more types in the future.
func KeyId(raw interface{}) ([]byte, error) {
	switch raw.(type) {
	case *ecdsa.PublicKey:
	case *rsa.PublicKey:
	default:
		return nil, fmt.Errorf("invalid key type: %T", raw)
	}

	// This is not standard; RFC allows any unique identifier as long as they
	// match in subject/authority chains but suggests specific hashing of DER
	// bytes of public key including DER tags.
	bs, err := x509.MarshalPKIXPublicKey(raw)
	if err != nil {
		return nil, err
	}

	kID := sha256.Sum256(bs)
	return kID[:], nil
}

// CalculateCertFingerprint calculates the SHA-1 fingerprint from the cert bytes.
func CalculateCertFingerprint(cert []byte) string {
	hash := sha1.Sum(cert)
	return HexString(hash[:])
}

// ParseCert parses the x509 certificate from a PEM-encoded value.
func ParseCert(pemValue string) (*x509.Certificate, error) {
	// The _ result below is not an error but the remaining PEM bytes.
	block, _ := pem.Decode([]byte(pemValue))
	if block == nil {
		return nil, fmt.Errorf("no PEM-encoded data found")
	}

	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("first PEM-block should be CERTIFICATE type")
	}

	return x509.ParseCertificate(block.Bytes)
}

// ParseLeafCerts parses all of the x509 certificates from a PEM-encoded value
// under the assumption that the first cert is a leaf (non-CA) cert and the
// rest are intermediate CA certs.
//
// If no certificates are found this returns an error.
func ParseLeafCerts(pemValue string) (*x509.Certificate, *x509.CertPool, error) {
	certs, err := parseCerts(pemValue)
	if err != nil {
		return nil, nil, err
	}

	leaf := certs[0]
	if leaf.IsCA {
		return nil, nil, fmt.Errorf("first PEM-block should be a leaf cert")
	}

	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		if !cert.IsCA {
			return nil, nil, fmt.Errorf("found an unexpected leaf cert after the first PEM-block")
		}
		intermediates.AddCert(cert)
	}

	return leaf, intermediates, nil
}

// ParseCerts parses the all x509 certificates from a PEM-encoded value.
// The first returned cert is a leaf cert and any other ones are intermediates.
//
// If no certificates are found this returns an error.
func parseCerts(pemValue string) ([]*x509.Certificate, error) {
	var out []*x509.Certificate

	rest := []byte(pemValue)
	for {
		// The _ result below is not an error but the remaining PEM bytes.
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining

		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("PEM-block should be CERTIFICATE type")
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, cert)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no PEM-encoded data found")
	}

	return out, nil
}
