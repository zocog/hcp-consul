// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/agent/consul/authmethod"
	"github.com/hashicorp/consul/agent/consul/state"
	"github.com/hashicorp/consul/agent/structs"
	pbauth "github.com/hashicorp/consul/proto-public/pbauth/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

// Login wraps the process of creating an ACLToken from the identity verified
// by an auth method.
type Login struct {
	binder *Binder
	writer *TokenWriter
	store  authmethod.Store
}

// NewLogin returns a new Login with the given binder and writer.
func NewLogin(binder *Binder, writer *TokenWriter, store authmethod.Store) *Login {
	return &Login{binder, writer, store}
}

// TokenForVerifiedIdentity creates an ACLToken for the given identity verified
// by an auth method.
func (l *Login) TokenForVerifiedIdentity(identity *authmethod.Identity, authMethod *structs.ACLAuthMethod, description string) (*structs.ACLToken, error) {
	bindings, err := l.binder.Bind(authMethod, identity)
	switch {
	case err != nil:
		return nil, err
	case bindings.None():
		// We try to prevent the creation of a useless token without taking a trip
		// through Raft and the state store if we can.
		return nil, acl.ErrPermissionDenied
	}

	token := &structs.ACLToken{
		Description:       description,
		Local:             authMethod.TokenLocality != "global", // TokenWriter prevents the creation of global tokens in secondary datacenters.
		AuthMethod:        authMethod.Name,
		ExpirationTTL:     authMethod.MaxTokenTTL,
		ServiceIdentities: bindings.ServiceIdentities,
		NodeIdentities:    bindings.NodeIdentities,
		Roles:             bindings.Roles,
		EnterpriseMeta:    bindings.EnterpriseMeta,
	}
	token.ACLAuthMethodEnterpriseMeta.FillWithEnterpriseMeta(&authMethod.EnterpriseMeta)

	updated, err := l.writer.Create(token, true)
	switch {
	case err != nil && strings.Contains(err.Error(), state.ErrTokenHasNoPrivileges.Error()):
		// If we were in a slight race with a role delete operation then we may
		// still end up failing to insert an unprivileged token in the state
		// machine instead. Return the same error as earlier so it doesn't
		// actually matter which one prevents the insertion.
		return nil, acl.ErrPermissionDenied
	case err != nil:
		return nil, err
	}
	return updated, nil
}

func (l *Login) TokenForWorkloadIdentity(ctx context.Context, identityResource *pbresource.Resource, bearerToken string, meta map[string]string) (*structs.ACLToken, error) {
	// First look up the auth method from the workload identity.
	// todo: need to build ent meta
	// todo: need to handle datacenter
	var workloadIdentity pbauth.WorkloadIdentity
	err := identityResource.Data.UnmarshalTo(&workloadIdentity)
	if err != nil {
		return nil, acl.PermissionDenied("could not unmarshal workload identity data", "name", identityResource.Id.Name)
	}
	if workloadIdentity.AuthMethodRef == nil {
		return nil, acl.PermissionDenied("workload identity does not have auth method set", "name", identityResource.Id.Name)
	}

	authMethod, validator, err := l.store.GetWithValidator(workloadIdentity.AuthMethodRef.Name, nil)
	switch {
	case errors.Is(err, acl.ErrNotFound):
		return nil, acl.PermissionDenied("auth method not found", "name", workloadIdentity.AuthMethodRef.Name)
	case err != nil:
		return nil, acl.PermissionDenied("failed to load auth method", "err", err.Error())
	}

	_, err = validator.ValidateLogin(ctx, bearerToken)
	if err != nil {
		// TODO(agentless): errors returned from validators aren't standardized so
		// it's hard to tell whether validation failed because of an invalid bearer
		// token or something internal/transient. We currently return Unauthenticated
		// for all errors because it's the most likely, but we should make validators
		// return a typed or sentinel error instead.
		return nil, acl.PermissionDenied("could not validate bearer token")
	}

	canBind, err := l.binder.CanBindToWorkloadIdentity(authMethod)
	switch {
	case err != nil:
		return nil, acl.PermissionDenied(err.Error())
	case !canBind:
		// We try to prevent the creation of a useless token without taking a trip
		// through Raft and the state store if we can.
		return nil, acl.PermissionDenied("auth method cannot bind to workload identities", "name", authMethod.Name)
	}
	descriptionPrefix := fmt.Sprintf("token created via login for %q", workloadIdentity.ForIdentity.String())
	description, err := BuildTokenDescription(descriptionPrefix, meta)
	if err != nil {
		return nil, err
	}

	token := &structs.ACLToken{
		Description:      description,
		Local:            authMethod.TokenLocality != "global", // TokenWriter prevents the creation of global tokens in secondary datacenters.
		AuthMethod:       authMethod.Name,
		ExpirationTTL:    authMethod.MaxTokenTTL,
		WorkloadIdentity: &structs.WorkloadIdentity{IdentityResource: identityResource},
		// todo: handle ent meta
	}
	token.ACLAuthMethodEnterpriseMeta.FillWithEnterpriseMeta(&authMethod.EnterpriseMeta)

	updated, err := l.writer.Create(token, true)
	switch {
	case err != nil && strings.Contains(err.Error(), state.ErrTokenHasNoPrivileges.Error()):
		// If we were in a slight race with a role delete operation then we may
		// still end up failing to insert an unprivileged token in the state
		// machine instead. Return the same error as earlier so it doesn't
		// actually matter which one prevents the insertion.
		return nil, acl.ErrPermissionDenied
	case err != nil:
		return nil, err
	}
	return updated, nil
}

// BuildTokenDescription builds a description for an ACLToken by encoding the
// given meta as JSON and applying the prefix.
func BuildTokenDescription(prefix string, meta map[string]string) (string, error) {
	if len(meta) == 0 {
		return prefix, nil
	}

	d, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s", prefix, d), nil
}
