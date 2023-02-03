// x-build !consulent

package acl

import "github.com/hashicorp/consul/sentinel"

// AuthorizerContext contains extra information that can be
// used in the determination of an ACL enforcement decision.
type AuthorizerContext struct {
	// Peer is the name of the peer that the resource was imported from.
	Peer string

	// Scope is a Sentinel-specific scope (Enterprise)
	Scope sentinel.ScopeFn

	// Namespace is the namespace of the requested resource (Enterprise).
	Namespace string

	// Partition is the partition of the requested resource (Enterprise).
	Partition string
}

func (c *AuthorizerContext) PeerOrEmpty() string {
	if c == nil {
		return ""
	}
	return c.Peer
}

// enterpriseAuthorizer stub interface
type enterpriseAuthorizer interface{}

func enforceEnterprise(_ Authorizer, _ Resource, _ string, _ string, _ *AuthorizerContext) (bool, EnforcementDecision, error) {
	return false, Deny, nil
}
