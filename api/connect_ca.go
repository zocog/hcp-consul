// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"fmt"
	"time"

	"github.com/mitchellh/mapstructure"
)

// CARoot represents a root CA certificate that is trusted.
type CARoot struct {
	// ID is a globally unique ID (UUID) representing this CA chain. It is
	// calculated from the SHA1 of the primary CA certificate.
	ID string

	// Name is a human-friendly name for this CA root. This value is
	// opaque to Consul and is not used for anything internally.
	Name string

	// SerialNumber is the x509 serial number of the primary CA certificate.
	SerialNumber uint64

	// SigningKeyID is the connect.HexString encoded id of the public key that
	// corresponds to the private key used to sign leaf certificates in the
	// local datacenter.
	//
	// The value comes from x509.Certificate.SubjectKeyId of the local leaf
	// signing cert.
	//
	// See https://www.rfc-editor.org/rfc/rfc3280#section-4.2.1.1 for more detail.
	SigningKeyID string

	// ExternalTrustDomain is the trust domain this root was generated under. It
	// is usually empty implying "the current cluster trust-domain". It is set
	// only in the case that a cluster changes trust domain and then all old roots
	// that are still trusted have the old trust domain set here.
	//
	// We currently DON'T validate these trust domains explicitly anywhere, see
	// IndexedRoots.TrustDomain doc. We retain this information for debugging and
	// future flexibility.
	ExternalTrustDomain string

	// NotBefore is the x509.Certificate.NotBefore value of the primary CA
	// certificate. This value should generally be a time in the past.
	NotBefore time.Time
	// NotAfter is the  x509.Certificate.NotAfter value of the primary CA
	// certificate. This is the time when the certificate will expire.
	NotAfter time.Time

	// RootCert is the PEM-encoded public certificate for the root CA. The
	// certificate is the same for all federated clusters.
	RootCert string

	// IntermediateCerts is a list of PEM-encoded intermediate certs to
	// attach to any leaf certs signed by this CA. The list may include a
	// certificate cross-signed by an old root CA, any subordinate CAs below the
	// root CA, and the intermediate CA used to sign leaf certificates in the
	// local Datacenter.
	//
	// If the provider which created this root uses an intermediate to sign
	// leaf certificates (Vault provider), or this is a secondary Datacenter then
	// the intermediate used to sign leaf certificates will be the last in the
	// list.
	IntermediateCerts []string

	// SigningCert is the PEM-encoded signing certificate and SigningKey
	// is the PEM-encoded private key for the signing certificate. These
	// may actually be empty if the CA plugin in use manages these for us.
	SigningCert string `json:",omitempty"`
	SigningKey  string `json:",omitempty"`

	// Active is true if this is the current active CA. This must only
	// be true for exactly one CA. For any method that modifies roots in the
	// state store, tests should be written to verify that multiple roots
	// cannot be active.
	Active bool

	// RotatedOutAt is the time at which this CA was removed from the state.
	// This will only be set on roots that have been rotated out from being the
	// active root.
	RotatedOutAt time.Time `json:"-"`

	// PrivateKeyType is the type of the private key used to sign certificates. It
	// may be "rsa" or "ec". This is provided as a convenience to avoid parsing
	// the public key to from the certificate to infer the type.
	PrivateKeyType string

	// PrivateKeyBits is the length of the private key used to sign certificates.
	// This is provided as a convenience to avoid parsing the public key from the
	// certificate to infer the type.
	PrivateKeyBits int

	CreateIndex uint64
	ModifyIndex uint64
}

func (c *CARoot) Clone() *CARoot {
	if c == nil {
		return nil
	}

	newCopy := *c
	newCopy.IntermediateCerts = cloneStringSlice(c.IntermediateCerts)
	return &newCopy
}

func cloneStringSlice(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// DeepCopy generates a deep copy of *CARoot
func (o *CARoot) DeepCopy() *CARoot {
	var cp CARoot = *o
	if o.IntermediateCerts != nil {
		cp.IntermediateCerts = make([]string, len(o.IntermediateCerts))
		copy(cp.IntermediateCerts, o.IntermediateCerts)
	}
	return &cp
}

// CARoots is a list of CARoot structures.
type CARoots []*CARoot

// Active returns the single CARoot that is marked as active, or nil if there
// is no active root (ex: when they are no roots).
func (c CARoots) Active() *CARoot {
	if c == nil {
		return nil
	}
	for _, r := range c {
		if r.Active {
			return r
		}
	}
	return nil
}

// CAConfig is the structure for the Connect CA configuration.
type CAConfig struct {
	// Provider is the CA provider implementation to use.
	Provider string

	// Configuration is arbitrary configuration for the provider. This
	// should only contain primitive values and containers (such as lists
	// and maps).
	Config map[string]interface{}

	// State is read-only data that the provider might have persisted for use
	// after restart or leadership transition. For example this might include
	// UUIDs of resources it has created. Setting this when writing a
	// configuration is an error.
	State map[string]string

	// ForceWithoutCrossSigning indicates that the CA reconfiguration should go
	// ahead even if the current CA is unable to cross sign certificates. This
	// risks temporary connection failures during the rollout as new leafs will be
	// rejected by proxies that have not yet observed the new root cert but is the
	// only option if a CA that doesn't support cross signing needs to be
	// reconfigured or mirated away from.
	ForceWithoutCrossSigning bool

	CreateIndex uint64
	ModifyIndex uint64
}

// CommonCAProviderConfig is the common options available to all CA providers.
type CommonCAProviderConfig struct {
	LeafCertTTL      time.Duration
	RootCertTTL      time.Duration
	SkipValidate     bool
	CSRMaxPerSecond  float32
	CSRMaxConcurrent int
}

// ConsulCAProviderConfig is the config for the built-in Consul CA provider.
type ConsulCAProviderConfig struct {
	CommonCAProviderConfig `mapstructure:",squash"`

	PrivateKey          string
	RootCert            string
	IntermediateCertTTL time.Duration
}

// ParseConsulCAConfig takes a raw config map and returns a parsed
// ConsulCAProviderConfig.
func ParseConsulCAConfig(raw map[string]interface{}) (*ConsulCAProviderConfig, error) {
	var config ConsulCAProviderConfig
	decodeConf := &mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		Result:           &config,
		WeaklyTypedInput: true,
	}

	decoder, err := mapstructure.NewDecoder(decodeConf)
	if err != nil {
		return nil, err
	}

	if err := decoder.Decode(raw); err != nil {
		return nil, fmt.Errorf("error decoding config: %s", err)
	}

	return &config, nil
}

// CARootList is the structure for the results of listing roots.
type CARootList struct {
	ActiveRootID string
	TrustDomain  string
	Roots        []*CARoot
}

// TODO(dans): remove me
//// CARoot represents a root CA certificate that is trusted.
//type CARoot struct {
//	// ID is a globally unique ID (UUID) representing this CA root.
//	ID string
//
//	// Name is a human-friendly name for this CA root. This value is
//	// opaque to Consul and is not used for anything internally.
//	Name string
//
//	// RootCertPEM is the PEM-encoded public certificate.
//	RootCertPEM string `json:"RootCert"`
//
//	// Active is true if this is the current active CA. This must only
//	// be true for exactly one CA. For any method that modifies roots in the
//	// state store, tests should be written to verify that multiple roots
//	// cannot be active.
//	Active bool
//
//	CreateIndex uint64
//	ModifyIndex uint64
//}

// LeafCert is a certificate that has been issued by a Connect CA.
type LeafCert struct {
	// SerialNumber is the unique serial number for this certificate.
	// This is encoded in standard hex separated by :.
	SerialNumber string

	// CertPEM and PrivateKeyPEM are the PEM-encoded certificate and private
	// key for that cert, respectively. This should not be stored in the
	// state store, but is present in the sign API response.
	CertPEM       string `json:",omitempty"`
	PrivateKeyPEM string `json:",omitempty"`

	// Service is the name of the service for which the cert was issued.
	// ServiceURI is the cert URI value.
	Service    string
	ServiceURI string

	// ValidAfter and ValidBefore are the validity periods for the
	// certificate.
	ValidAfter  time.Time
	ValidBefore time.Time

	CreateIndex uint64
	ModifyIndex uint64
}

// CARoots queries the list of available roots.
func (h *Connect) CARoots(q *QueryOptions) (*CARootList, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/connect/ca/roots")
	r.setQueryOptions(q)
	rtt, resp, err := h.c.doRequest(r)
	if err != nil {
		return nil, nil, err
	}
	defer closeResponseBody(resp)
	if err := requireOK(resp); err != nil {
		return nil, nil, err
	}

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out CARootList
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return &out, qm, nil
}

// CAGetConfig returns the current CA configuration.
func (h *Connect) CAGetConfig(q *QueryOptions) (*CAConfig, *QueryMeta, error) {
	r := h.c.newRequest("GET", "/v1/connect/ca/configuration")
	r.setQueryOptions(q)
	rtt, resp, err := h.c.doRequest(r)
	if err != nil {
		return nil, nil, err
	}
	defer closeResponseBody(resp)
	if err := requireOK(resp); err != nil {
		return nil, nil, err
	}

	qm := &QueryMeta{}
	parseQueryMeta(resp, qm)
	qm.RequestTime = rtt

	var out CAConfig
	if err := decodeBody(resp, &out); err != nil {
		return nil, nil, err
	}
	return &out, qm, nil
}

// CASetConfig sets the current CA configuration.
func (h *Connect) CASetConfig(conf *CAConfig, q *WriteOptions) (*WriteMeta, error) {
	r := h.c.newRequest("PUT", "/v1/connect/ca/configuration")
	r.setWriteOptions(q)
	r.obj = conf
	rtt, resp, err := h.c.doRequest(r)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)
	if err := requireOK(resp); err != nil {
		return nil, err
	}

	wm := &WriteMeta{}
	wm.RequestTime = rtt
	return wm, nil
}
