// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package types

import (
	"github.com/hashicorp/consul/internal/resource"
	pbauth "github.com/hashicorp/consul/proto-public/pbauth/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

const (
	IntentionsKind = "Intentions"
)

var (
	IntentionsV1Alpha1Type = &pbresource.Type{
		Group:        GroupName,
		GroupVersion: CurrentVersion,
		Kind:         IntentionsKind,
	}

	IntentionsType = IntentionsV1Alpha1Type
)

func RegisterIntentions(r resource.Registry) {
	r.Register(resource.Registration{
		Type:     IntentionsType,
		Proto:    &pbauth.Intentions{},
		Validate: nil,
	})
}
