// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package types

import (
	"github.com/hashicorp/consul/internal/resource"
	pbauth "github.com/hashicorp/consul/proto-public/pbauth/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

const (
	WorkloadIdentityKind = "WorkloadIdentity"
)

var (
	WorkloadIdentityV1Alpha1Type = &pbresource.Type{
		Group:        GroupName,
		GroupVersion: CurrentVersion,
		Kind:         WorkloadIdentityKind,
	}

	WorkloadIdentityType = WorkloadIdentityV1Alpha1Type
)

func RegisterWorkloadIdentity(r resource.Registry) {
	r.Register(resource.Registration{
		Type:     WorkloadIdentityType,
		Proto:    &pbauth.WorkloadIdentity{},
		Validate: nil,
	})
}
