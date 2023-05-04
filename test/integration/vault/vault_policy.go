package test

import "fmt"

// Standard recommended Policy for Vault Managed PKI CAs
func policyVaultMg(root, intr string) string {
	return fmt.Sprintf(vaultMgTmpl, root, intr)
}

// Standard recommended Policy for Consul Managed PKI CAs
func policyConsulMg(root, intr string) string {
	return fmt.Sprintf(consulMgTmpl, root, intr)
}

const consulMgTmpl = `
path "/sys/mounts/%[1]s" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "/sys/mounts/%[2]s" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "/sys/mounts/%[2]s/tune" {
  capabilities = [ "update" ]
}

path "/%[1]s/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "/%[2]s/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "auth/token/renew-self" {
  capabilities = [ "update" ]
}

path "auth/token/lookup-self" {
  capabilities = [ "read" ]
}
`

const vaultMgTmpl = `
path "/sys/mounts/%[1]s" {
  capabilities = [ "read" ]
}

path "/sys/mounts/%[2]s" {
  capabilities = [ "read" ]
}

path "/sys/mounts/%[2]s/tune" {
  capabilities = [ "update" ]
}

path "/%[1]s/*" {
  capabilities = [ "read", "update" ]
}

path "%[1]s/root/sign-intermediate" {
  capabilities = [ "update" ]
}

path "%[2]s/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "auth/token/renew-self" {
  capabilities = [ "update" ]
}

path "auth/token/lookup-self" {
  capabilities = [ "read" ]
}
`
