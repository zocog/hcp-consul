package test

import (
	"fmt"
	"os/exec"
	"testing"

	vapi "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

func TestTutorial(t *testing.T) {
	vault := NewTestVaultServer(t, "vault", "local")
	defer vault.Stop()
	consul := NewTestConsulServer(t, "consul", "local")
	defer consul.Stop()

	t.Run("demo", func(t *testing.T) {
		tutorial(t, consul, vault)
	})
}

// Vault as a Consul Service Mesh Certificate Authority demo in code
func tutorial(t *testing.T, c TestConsulServer, v TestVaultServer) {
	c.flagForRestart()
	const (
		policyName = "ca"
		rootName   = "connect_root"
		intrName   = "connect_dc1_inter"
		leafName   = "leaf"
	)
	// vault setup
	err := v.Client().Sys().Mount(rootName+"/", &vapi.MountInput{Type: "pki"})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := v.Client().Sys().Unmount(rootName + "/")
		require.NoError(t, err)
	})
	err = v.Client().Sys().Mount(intrName+"/", &vapi.MountInput{Type: "pki"})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := v.Client().Sys().Unmount(intrName + "/")
		require.NoError(t, err)
	})
	err = v.Client().Sys().PutPolicy(policyName, policyVaultMg(rootName, intrName))
	require.NoError(t, err)
	t.Cleanup(func() {
		err := v.Client().Sys().DeletePolicy(policyName)
		require.NoError(t, err)
	})
	secret, err := v.Client().Auth().Token().Create(
		&vapi.TokenCreateRequest{Policies: []string{policyName}})
	token := secret.Auth.ClientToken
	require.NoError(t, err)

	// consul setup
	_, err = c.Client().Connect().CASetConfig(
		caConfReq(v.Addr, token, rootName, intrName), nil)
	require.NoError(t, err)
	// can't undo this... maybe add note that new tests that touch this
	// will need to overwrite other setups

	// tests
	caconf, _, err := c.Client().Connect().CAGetConfig(nil)
	require.NoError(t, err)
	require.Equal(t, caconf.Provider, "vault")
	roots, _, err := c.Client().Agent().ConnectCARoots(nil)
	require.NoError(t, err)
	require.Len(t, roots.Roots, 2)
	leaf, _, err := c.Client().Agent().ConnectCALeaf(leafName, nil)
	require.NoError(t, err)
	certpem1 := leaf.CertPEM
	require.Contains(t, certpem1, "CERTIFICATE")
	require.Contains(t, leaf.PrivateKeyPEM, "PRIVATE")
	leaf, _, err = c.Client().Agent().ConnectCALeaf(leafName, nil)
	require.NoError(t, err)
	certpem2 := leaf.CertPEM
	require.Contains(t, certpem2, "CERTIFICATE")
	require.Contains(t, leaf.PrivateKeyPEM, "PRIVATE")
	require.Equal(t, certpem1, certpem2)

	// curlTests(t, c, v)
}

// tutorial's curl commands
func curlTests(t *testing.T, c TestConsulServer, v TestVaultServer) {
	out, err := exec.Command("curl", "-s", "-verbose", "--header",
		"X-Consul-Token: "+v.RootToken,
		c.HTTPAddr+"/v1/agent/connect/ca/leaf/leaf").CombinedOutput()
	require.NoError(t, err)
	fmt.Printf("%s\n", out)
	out, err = exec.Command("curl", "-s", "-verbose", "--header",
		"X-Consul-Token: "+v.RootToken,
		c.HTTPAddr+"/v1/agent/connect/ca/leaf/leaf").CombinedOutput()
	require.NoError(t, err)
	fmt.Printf("%s\n", out)
}
