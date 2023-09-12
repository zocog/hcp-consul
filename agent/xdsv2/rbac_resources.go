// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package xdsv2

import (
	"fmt"

	envoy_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_network_rbac_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	"github.com/hashicorp/consul/agent/xds/response"
	"github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1/pbproxystate"
)

const (
	baseL4PermissionKey = "consul-intentions-layer4"
)

func MakeL4RBAC(trafficPermissions *pbproxystate.L4TrafficPermissions) ([]*envoy_listener_v3.Filter, error) {
	if trafficPermissions == nil {
		return nil, nil
	}

	var result []*envoy_listener_v3.Filter

	// First, compute any "deny" policies as they should be applied first.
	denyPolicies := makeRBACPolicies(trafficPermissions.DenyPermissions)

	if len(denyPolicies) > 0 {
		denyRBAC := &envoy_rbac_v3.RBAC{
			Action:   envoy_rbac_v3.RBAC_DENY,
			Policies: denyPolicies,
		}
		denyRBACFilter, err := makeL4RBACFilter(denyRBAC)
		if err != nil {
			return nil, err
		}

		result = append(result, denyRBACFilter)
	}

	// Next, compute "allow" policies.
	allowPolicies := makeRBACPolicies(trafficPermissions.AllowPermissions)

	// Handle consul's default policy.
	//
	// If consul is in default deny and there are no allow policies, and so
	// we need to add an allow-nothing filter so that everything that doesn't match
	// any of the deny policies will still get denied.
	//
	// If consul is in default allow, we don't care as this is an insecure setting,
	// and so we just let whatever traffic permissions you have apply without applying any default filters.
	// (note if no traffic permissions exist, no filter means allow all which is what we want).
	if trafficPermissions.DefaultAction == pbproxystate.TrafficPermissionAction_TRAFFIC_PERMISSION_ACTION_DENY &&
		len(allowPolicies) == 0 {

		// An empty RBAC object will match on nothing and will default to action "ALLOW,"
		// which is equivalent to having an "allow-nothing" filter.
		allowRBACFilter, err := makeL4RBACFilter(&envoy_rbac_v3.RBAC{})
		if err != nil {
			return nil, err
		}
		result = append(result, allowRBACFilter)
		return result, err
	}

	// Otherwise, we will create an "allow" RBAC filter with provided policies.
	if len(allowPolicies) > 0 {
		allowRBAC := &envoy_rbac_v3.RBAC{
			Action:   envoy_rbac_v3.RBAC_ALLOW,
			Policies: allowPolicies,
		}
		allowRBACFilter, err := makeL4RBACFilter(allowRBAC)
		if err != nil {
			return nil, err
		}
		result = append(result, allowRBACFilter)
	}

	return result, nil
}

func makeL4RBACFilter(rbac *envoy_rbac_v3.RBAC) (*envoy_listener_v3.Filter, error) {
	cfg := &envoy_network_rbac_v3.RBAC{
		StatPrefix: "connect_authz",
		Rules:      rbac,
	}
	return makeEnvoyFilter("envoy.filters.network.rbac", cfg)
}

func makeRBACPolicies(l4Permissions []*pbproxystate.L4Permission) map[string]*envoy_rbac_v3.Policy {
	policyLabel := func(i int) string {
		if len(l4Permissions) == 1 {
			return baseL4PermissionKey
		}
		return fmt.Sprintf("%s-%d", baseL4PermissionKey, i)
	}

	policies := make(map[string]*envoy_rbac_v3.Policy)

	for i, permission := range l4Permissions {
		policies[policyLabel(i)] = makeRBACPolicy(permission)
	}

	return policies
}

//func makeRBACs(trafficPermissions *pbproxystate.L4TrafficPermissions) ([]*envoy_rbac_v3.RBAC, error) {
//	allowRBAC := &envoy_rbac_v3.RBAC{
//		Action:   envoy_rbac_v3.RBAC_ALLOW,
//		Policies: make(map[string]*envoy_rbac_v3.Policy),
//	}
//
//	denyRBAC := &envoy_rbac_v3.RBAC{
//		Action:   envoy_rbac_v3.RBAC_DENY,
//		Policies: make(map[string]*envoy_rbac_v3.Policy),
//	}
//
//	policyLabel := func(i int) string {
//		if len(trafficPermissions.Permissions) == 1 {
//			return baseL4PermissionKey
//		}
//		return fmt.Sprintf("%s-%d", baseL4PermissionKey, i)
//	}
//
//	for i, p := range trafficPermissions.Permissions {
//		allowPolicy, err := makeRBACPolicy(p.AllowPrincipals)
//		if err != nil {
//			return nil, err
//		}
//
//		if allowPolicy != nil {
//			allowRBAC.Policies[policyLabel(i)] = allowPolicy
//		}
//
//		denyPolicy, err := makeRBACPolicy(p.DenyPrincipals)
//		if err != nil {
//			return nil, err
//		}
//
//		if denyPolicy != nil {
//			denyRBAC.Policies[policyLabel(i)] = denyPolicy
//		}
//	}
//
//	var rbacs []*envoy_rbac_v3.RBAC
//	if rbac := finalizeRBAC(allowRBAC, trafficPermissions.DefaultAction); rbac != nil {
//		rbacs = append(rbacs, rbac)
//	}
//
//	if rbac := finalizeRBAC(denyRBAC, trafficPermissions.DefaultAction); rbac != nil {
//		rbacs = append(rbacs, rbac)
//	}
//
//	return rbacs, nil
//}

func finalizeRBAC(rbac *envoy_rbac_v3.RBAC, defaultAction pbproxystate.TrafficPermissionAction) *envoy_rbac_v3.RBAC {
	isRBACAllow := rbac.Action == envoy_rbac_v3.RBAC_ALLOW
	isConsulAllow := defaultAction == pbproxystate.TrafficPermissionAction_TRAFFIC_PERMISSION_ACTION_ALLOW
	// Remove allow traffic permissions with default allow. This is required because including an allow RBAC filter enforces default deny.
	// It is safe because deny traffic permissions are applied before allow permissions, so explicit allow is equivalent to default allow.
	removeAllows := isRBACAllow && isConsulAllow
	if removeAllows {
		return nil
	}

	if len(rbac.Policies) != 0 {
		return rbac
	}

	// Include an empty allow RBAC filter to enforce Consul's default deny.
	includeEmpty := isRBACAllow && !isConsulAllow
	if includeEmpty {
		return rbac
	}

	return nil
}

func makeRBACPolicy(l4Permission *pbproxystate.L4Permission) *envoy_rbac_v3.Policy {
	if l4Permission == nil {
		return nil
	}

	policy := &envoy_rbac_v3.Policy{}

	// Collect all principals.
	var principals []*envoy_rbac_v3.Principal

	for _, l4Principal := range l4Permission.Principals {
		principals = append(principals, toEnvoyPrincipal(l4Principal.ToL7Principal()))
	}

	// Lastly, allow any permission because it's an l4 policy.
	policy.Permissions = []*envoy_rbac_v3.Permission{anyPermission()}

	return policy
}

func toEnvoyPrincipal(p *pbproxystate.L7Principal) *envoy_rbac_v3.Principal {
	orIDs := make([]*envoy_rbac_v3.Principal, 0, len(p.Spiffes))
	for _, regex := range p.Spiffes {
		orIDs = append(orIDs, principal(regex.Regex, false, regex.Xfcc))
	}

	includePrincipal := orPrincipals(orIDs)

	if len(p.ExcludeSpiffes) == 0 {
		return includePrincipal
	}

	excludePrincipals := make([]*envoy_rbac_v3.Principal, 0, len(p.ExcludeSpiffes))
	for _, sid := range p.ExcludeSpiffes {
		excludePrincipals = append(excludePrincipals, principal(sid.Regex, true, sid.Xfcc))
	}
	excludePrincipal := orPrincipals(excludePrincipals)

	return andPrincipals([]*envoy_rbac_v3.Principal{includePrincipal, excludePrincipal})
}

func principal(spiffeID string, negate, xfcc bool) *envoy_rbac_v3.Principal {
	var p *envoy_rbac_v3.Principal
	if xfcc {
		p = xfccPrincipal(spiffeID)
	} else {
		p = idPrincipal(spiffeID)
	}

	if !negate {
		return p
	}

	return negatePrincipal(p)
}

func negatePrincipal(p *envoy_rbac_v3.Principal) *envoy_rbac_v3.Principal {
	return &envoy_rbac_v3.Principal{
		Identifier: &envoy_rbac_v3.Principal_NotId{
			NotId: p,
		},
	}
}

func idPrincipal(spiffeID string) *envoy_rbac_v3.Principal {
	return &envoy_rbac_v3.Principal{
		Identifier: &envoy_rbac_v3.Principal_Authenticated_{
			Authenticated: &envoy_rbac_v3.Principal_Authenticated{
				PrincipalName: &envoy_matcher_v3.StringMatcher{
					MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
						SafeRegex: response.MakeEnvoyRegexMatch(spiffeID),
					},
				},
			},
		},
	}
}

func andPrincipals(ids []*envoy_rbac_v3.Principal) *envoy_rbac_v3.Principal {
	switch len(ids) {
	case 1:
		return ids[0]
	default:
		return &envoy_rbac_v3.Principal{
			Identifier: &envoy_rbac_v3.Principal_AndIds{
				AndIds: &envoy_rbac_v3.Principal_Set{
					Ids: ids,
				},
			},
		}
	}
}

func orPrincipals(ids []*envoy_rbac_v3.Principal) *envoy_rbac_v3.Principal {
	switch len(ids) {
	case 1:
		return ids[0]
	default:
		return &envoy_rbac_v3.Principal{
			Identifier: &envoy_rbac_v3.Principal_OrIds{
				OrIds: &envoy_rbac_v3.Principal_Set{
					Ids: ids,
				},
			},
		}
	}
}

func anyPermission() *envoy_rbac_v3.Permission {
	return &envoy_rbac_v3.Permission{
		Rule: &envoy_rbac_v3.Permission_Any{Any: true},
	}
}

func xfccPrincipal(spiffeID string) *envoy_rbac_v3.Principal {
	return &envoy_rbac_v3.Principal{
		Identifier: &envoy_rbac_v3.Principal_Header{
			Header: &envoy_route_v3.HeaderMatcher{
				Name: "x-forwarded-client-cert",
				HeaderMatchSpecifier: &envoy_route_v3.HeaderMatcher_StringMatch{
					StringMatch: &envoy_matcher_v3.StringMatcher{
						MatchPattern: &envoy_matcher_v3.StringMatcher_SafeRegex{
							SafeRegex: response.MakeEnvoyRegexMatch(spiffeID),
						},
					},
				},
			},
		},
	}
}
