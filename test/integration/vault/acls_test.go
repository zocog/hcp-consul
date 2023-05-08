package test

import (
	"testing"

	vapi "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

// Test the recommended ACLs from the documentation
// the doc: ../../../website/content/docs/connect/ca/vault.mdx
func TestACLs(t *testing.T) {
	vault := NewTestVaultServer(t, "vault", "local")
	consul := NewTestConsulServer(t, "consul", "local")

	t.Run("consul-managed-acls", func(t *testing.T) {
		testConsulManagedACLs(t, consul, vault)
	})

	if consul.needsRestart() {
		t.Log("Restarting Consul")
		consul.Stop()
		consul = NewTestConsulServer(t, "consul", "local")
	}

	if vault.needsRestart() {
		t.Log("Restarting Vault")
		vault.Stop()
		vault = NewTestVaultServer(t, "vault", "local")
	}

	t.Run("vault-managed-acls", func(t *testing.T) {
		testVaultManagedACLs(t, consul, vault)
	})

	consul.Stop()
	vault.Stop()
}

func testConsulManagedACLs(t *testing.T, c TestConsulServer, v TestVaultServer) {
	t.Helper()
	c.flagForRestart()
	// setup
	policyName := "consul_managed"
	rootName := "acls_test_cm_root"
	intrName := "acls_test_cm_intr"
	policy := policyConsulMg(rootName, intrName)

	// policy/token
	err := v.Client().Sys().PutPolicy(policyName, policy)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := v.Client().Sys().DeletePolicy(policyName)
		require.NoError(t, err)
	})
	secret, err := v.Client().Auth().Token().Create(
		&vapi.TokenCreateRequest{Policies: []string{policyName}})
	require.NoError(t, err)

	token := secret.Auth.ClientToken
	// test
	// NOTE: can't clean this up, requires restart
	_, err = c.Client().Connect().CASetConfig(
		caConfReq(v.Addr, token, rootName, intrName), nil)
	require.NoError(t, err)
	_, err = c.Client().Connect().CASetConfig(
		caConfReq(v.Addr, "bad-token", rootName, intrName), nil)
	require.Error(t, err)

	// Cleanup Vault side of CASetConfig call
	t.Cleanup(func() {
		err := v.Client().Sys().Unmount(rootName + "/")
		require.NoError(t, err)
	})
	t.Cleanup(func() {
		err := v.Client().Sys().Unmount(intrName + "/")
		require.NoError(t, err)
	})

	// check that it took and certs exist
	caconf, _, err := c.Client().Connect().CAGetConfig(nil)
	require.NoError(t, err)
	require.Equal(t, "vault", caconf.Provider)
	roots, _, err := c.Client().Agent().ConnectCARoots(nil)
	require.NoError(t, err)
	require.Len(t, roots.Roots, 2)
}

func testVaultManagedACLs(t *testing.T, c TestConsulServer, v TestVaultServer) {
	t.Helper()
	c.flagForRestart()
	// setup
	policyName := "vault_managed"
	rootName := "acls_test_vm_root"
	intrName := "acls_test_vm_intr"
	policy := policyVaultMg(rootName, intrName)

	// vault managed means vault creates the certs
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
	// policy/token
	err = v.Client().Sys().PutPolicy(policyName, policy)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := v.Client().Sys().DeletePolicy(policyName)
		require.NoError(t, err)
	})
	secret, err := v.Client().Auth().Token().Create(
		&vapi.TokenCreateRequest{Policies: []string{policyName}})
	require.NoError(t, err)

	token := secret.Auth.ClientToken

	// test
	// NOTE: can't clean this up, requires restart
	_, err = c.Client().Connect().CASetConfig(
		caConfReq(v.Addr, token, rootName, intrName), nil)
	require.NoError(t, err)
	_, err = c.Client().Connect().CASetConfig(
		caConfReq(v.Addr, "bad-token", rootName, intrName), nil)
	require.Error(t, err)

	// check that it took and certs exist
	caconf, _, err := c.Client().Connect().CAGetConfig(nil)
	require.NoError(t, err)
	require.Equal(t, "vault", caconf.Provider)
	roots, _, err := c.Client().Agent().ConnectCARoots(nil)
	require.NoError(t, err)
	require.Len(t, roots.Roots, 2)
}
