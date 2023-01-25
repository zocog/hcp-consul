// x-build !consulent

package structs

import (
	"github.com/hashicorp/consul/acl"
)

func validateSourceIntentionEnterpriseMeta(_, _ *acl.EnterpriseMeta) error {
	return nil
}
