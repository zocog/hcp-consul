// x-build !consulent

package agent

import (
	"net/http"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/agent/structs"
)

func (s *HTTPHandlers) parseEntMeta(req *http.Request, entMeta *acl.EnterpriseMeta) error {
	return s.entMetaHelper.ParseEntMeta(req, entMeta)
}

func (s *HTTPHandlers) validateEnterpriseIntentionPartition(logName, partition string) error {
	return s.entMetaHelper.ValidateEnterpriseIntentionPartition(logName, partition)
}

func (s *HTTPHandlers) validateEnterpriseIntentionNamespace(logName, ns string, wildOK bool) error {
	return s.entMetaHelper.ValidateEnterpriseIntentionNamespace(logName, ns, wildOK)
}

func (s *HTTPHandlers) parseEntMetaNoWildcard(req *http.Request, entMeta *acl.EnterpriseMeta) error {
	return s.entMetaHelper.ParseEntMetaNoWildcard(req, entMeta)
}

func (s *HTTPHandlers) rewordUnknownEnterpriseFieldError(err error) error {
	return s.entMetaHelper.RewordUnknownEnterpriseFieldError(err)
}

func (s *HTTPHandlers) parseACLAuthMethodEnterpriseMeta(req *http.Request, meta *structs.ACLAuthMethodEnterpriseMeta) error {
	return s.entMetaHelper.ParseACLAuthMethodEnterpriseMeta(req, meta)
}

// enterpriseHandler is a noop for the enterprise implementation. we pass the original back
func (s *HTTPHandlers) enterpriseHandler(next http.Handler) http.Handler {
	return s.entMetaHelper.EnterpriseHandler(next)
}

// uiTemplateDataTransform returns an optional uiserver.UIDataTransform to allow
// altering UI data in enterprise.
func (s *HTTPHandlers) uiTemplateDataTransform(data map[string]interface{}) error {
	return s.entMetaHelper.UITemplateDataTransform(data)
}

func (s *HTTPHandlers) parseEntMetaPartition(req *http.Request, meta *acl.EnterpriseMeta) error {
	return s.entMetaHelper.ParseEntMetaPartition(req, meta)
}
