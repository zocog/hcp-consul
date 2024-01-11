// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

//go:build !consulent

package discovery

import "github.com/hashicorp/consul/acl"

func getRequestEnterpriseMetaFromQueryPayload(req *QueryPayload) acl.EnterpriseMeta {
	return acl.EnterpriseMeta{}
}
