// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mesh

import (
	"github.com/hashicorp/consul/internal/auth/internal/types"
	"github.com/hashicorp/consul/internal/resource"
)

var (
	// API Group Information

	APIGroup        = types.GroupName
	VersionV1Alpha1 = types.VersionV1Alpha1
	CurrentVersion  = types.CurrentVersion

	// Resource Kind Names.

	IntentionsKind       = types.IntentionsKind
	WorkloadIdentityKind = types.WorkloadIdentityKind

	// Resource Types for the v1alpha1 version.

	IntentionsV1Alpha1Type       = types.IntentionsV1Alpha1Type
	WorkloadIdentityV1Alpha1Type = types.WorkloadIdentityV1Alpha1Type
)

// RegisterTypes adds all resource types within the "catalog" API group
// to the given type registry
func RegisterTypes(r resource.Registry) {
	types.Register(r)
}
