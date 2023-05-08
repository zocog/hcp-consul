package test

import (
	"testing"

	vapi "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

func TestCaConf(t *testing.T) {
	vault := NewTestVaultServer(t, "vault", "local")
	consul := NewTestConsulServer(t, "consul", "local")

	t.Run("ConsulACLs", func(t *testing.T) {
		testCaConf(t, consul, vault)
	})

	consul.Stop()
	vault.Stop()
}

func testCaConf(t *testing.T, c TestConsulServer, v TestVaultServer) {
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
	t.Cleanup(func() {
		err := v.Client().Auth().Token().RevokeTree(token)
		require.NoError(t, err)
	})

	// test
	// NOTE: can't clean this up, requires restart
	// set then get to test
	// first set minimal required for it to work
	caconfReq := caConfReq(v.Addr, token, rootName, intrName)
	_, err = c.Client().Connect().CASetConfig(caconfReq, nil)
	require.NoError(t, err)
	caconfReqGet, _, err := c.Client().Connect().CAGetConfig(nil)
	require.NoError(t, err)
	require.Equal(t, caconfReq.Config, caconfReqGet.Config)

	// next set all options (sans auth-method) to make sure they take
	caconfAll := caConfAll(v.Addr, token, rootName, intrName)
	_, err = c.Client().Connect().CASetConfig(caconfAll, nil)
	require.NoError(t, err)
	caconfAllGet, _, err := c.Client().Connect().CAGetConfig(nil)
	require.NoError(t, err)

	require.Equal(t, caconfAll.Config, caconfAllGet.Config)

	// Cleanup Vault side of CASetConfig call
	t.Cleanup(func() {
		err := v.Client().Sys().Unmount(rootName + "/")
		require.NoError(t, err)
	})
	t.Cleanup(func() {
		err := v.Client().Sys().Unmount(intrName + "/")
		require.NoError(t, err)
	})

}
