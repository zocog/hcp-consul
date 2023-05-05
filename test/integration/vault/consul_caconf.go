package test

import (
	"path/filepath"

	capi "github.com/hashicorp/consul/api"
)

// basica ca-conf for testing
func caConf(addr, token, rootPath, intrPath string) *capi.CAConfig {
	return &capi.CAConfig{
		Provider: "vault",
		Config: map[string]any{
			"Address":             addr,
			"Token":               token,
			"RootPKIPath":         rootPath,
			"IntermediatePKIPath": intrPath,
		},
	}
}

// All CA options except for auth-method as it requires external services
func caConfAll(addr, token, rootPath, intrPath string) *capi.CAConfig {
	certFile, err := filepath.Abs("./testdata/cert.pem")
	if err != nil {
		panic("failed to open test cert file:" + err.Error())
	}
	keyFile, err := filepath.Abs("./testdata/key.pem")
	if err != nil {
		panic("failed to open test key file:" + err.Error())
	}
	return &capi.CAConfig{
		Provider: "vault",
		Config: map[string]any{
			// base
			"Address":                  addr,
			"Token":                    token,
			"RootPKIPath":              rootPath,
			"RootPKINamespace":         "foo",
			"IntermediatePKIPath":      intrPath,
			"IntermediatePKINamespace": "foo",
			"CAFile":                   certFile,
			"CAPath":                   "./testdata",
			"CertFile":                 certFile,
			"KeyFile":                  keyFile,
			"TLSServerName":            "foo",
			"TLSSkipVerify":            true,
			"Namespace":                "foo",
			// common
			"CSRMaxConcurrent":    2.0,
			"CSRMaxPerSecond":     2.0,
			"LeafCertTTL":         "73h",
			"RootCertTTL":         "87601h",
			"IntermediateCertTTL": "8761h",
			"PrivateKeyType":      "ec",
			"PrivateKeyBits":      256.0,
		},
	}
}
