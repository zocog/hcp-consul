package agent

import (
	"fmt"
	"github.com/hashicorp/serf/serf"
	"net/http"
	"strings"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/consul/api"
)

// HTTPHandler from http_oss.go
type EnterpriseMetaHelper interface {
	SetAgent(agent *Agent)
	GetAgent() *Agent
	SerfMemberFillAuthzContext(m *serf.Member, ctx *acl.AuthorizerContext)
	AgentServiceFillAuthzContext(s *api.AgentService, ctx *acl.AuthorizerContext)
	EnterpriseHandler(next http.Handler) http.Handler
	UITemplateDataTransform(data map[string]interface{}) error
	ParseACLAuthMethodEnterpriseMeta(req *http.Request, aclAuthMethodEntMeta *structs.ACLAuthMethodEnterpriseMeta) error
	ParseEntMeta(req *http.Request, entMeta *acl.EnterpriseMeta) error
	ParseEntMetaPartition(req *http.Request, meta *acl.EnterpriseMeta) error
	ParseEntMetaNoWildcard(req *http.Request, meta *acl.EnterpriseMeta) error
	RewordUnknownEnterpriseFieldError(err error) error
	ValidateEnterpriseIntentionPartition(logName, partition string) error
	ValidateEnterpriseIntentionNamespace(logName, ns string, wildOK bool) error
}

type EnterpriseMetaHelperOSS struct {
	agent *Agent
}

func (s *EnterpriseMetaHelperOSS) SetAgent(agent *Agent) {
	s.agent = agent
}

func (s *EnterpriseMetaHelperOSS) GetAgent() *Agent {
	return s.agent
}

func (s *EnterpriseMetaHelperOSS) ParseEntMeta(req *http.Request, entMeta *acl.EnterpriseMeta) error {
	if headerNS := req.Header.Get("X-Consul-Namespace"); headerNS != "" {
		return HTTPError{
			StatusCode: http.StatusBadRequest,
			Reason:     "Invalid header: \"X-Consul-Namespace\" - Namespaces are a Consul Enterprise feature",
		}
	}
	if queryNS := req.URL.Query().Get("ns"); queryNS != "" {
		return HTTPError{
			StatusCode: http.StatusBadRequest,
			Reason:     "Invalid query parameter: \"ns\" - Namespaces are a Consul Enterprise feature",
		}
	}

	return s.ParseEntMetaPartition(req, entMeta)
}

func (s *EnterpriseMetaHelperOSS) ValidateEnterpriseIntentionPartition(logName, partition string) error {
	if partition == "" {
		return nil
	} else if strings.ToLower(partition) == "default" {
		return nil
	}

	// No special handling for wildcard namespaces as they are pointless in OSS.

	return HTTPError{
		StatusCode: http.StatusBadRequest,
		Reason:     "Invalid " + logName + "(" + partition + ")" + ": Partitions is a Consul Enterprise feature",
	}
}

func (s *EnterpriseMetaHelperOSS) ValidateEnterpriseIntentionNamespace(logName, ns string, _ bool) error {
	if ns == "" {
		return nil
	} else if strings.ToLower(ns) == structs.IntentionDefaultNamespace {
		return nil
	}

	// No special handling for wildcard namespaces as they are pointless in OSS.

	return HTTPError{
		StatusCode: http.StatusBadRequest,
		Reason:     "Invalid " + logName + "(" + ns + ")" + ": Namespaces is a Consul Enterprise feature",
	}
}

func (s *EnterpriseMetaHelperOSS) ParseEntMetaNoWildcard(req *http.Request, _ *acl.EnterpriseMeta) error {
	return s.ParseEntMeta(req, nil)
}

func (s *EnterpriseMetaHelperOSS) RewordUnknownEnterpriseFieldError(err error) error {
	if err == nil {
		return nil
	}

	msg := err.Error()

	if strings.Contains(msg, "json: unknown field ") {
		quotedField := strings.TrimPrefix(msg, "json: unknown field ")

		switch quotedField {
		case `"Namespace"`:
			return fmt.Errorf("%v - Namespaces are a Consul Enterprise feature", err)
		}
	}

	return err
}

func (s *EnterpriseMetaHelperOSS) ParseACLAuthMethodEnterpriseMeta(req *http.Request, _ *structs.ACLAuthMethodEnterpriseMeta) error {
	if methodNS := req.URL.Query().Get("authmethod-ns"); methodNS != "" {
		return HTTPError{
			StatusCode: http.StatusBadRequest,
			Reason:     "Invalid query parameter: \"authmethod-ns\" - Namespaces are a Consul Enterprise feature",
		}
	}

	return nil
}

// enterpriseHandler is a noop for the enterprise implementation. we pass the original back
func (s *EnterpriseMetaHelperOSS) EnterpriseHandler(next http.Handler) http.Handler {
	return next
}

// uiTemplateDataTransform returns an optional uiserver.UIDataTransform to allow
// altering UI data in enterprise.
func (s *EnterpriseMetaHelperOSS) UITemplateDataTransform(data map[string]interface{}) error {
	return nil
}

func (s *EnterpriseMetaHelperOSS) ParseEntMetaPartition(req *http.Request, meta *acl.EnterpriseMeta) error {
	if headerAP := req.Header.Get("X-Consul-Partition"); headerAP != "" {
		return HTTPError{
			StatusCode: http.StatusBadRequest,
			Reason:     "Invalid header: \"X-Consul-Partition\" - Partitions are a Consul Enterprise feature",
		}
	}
	if queryAP := req.URL.Query().Get("partition"); queryAP != "" {
		return HTTPError{
			StatusCode: http.StatusBadRequest,
			Reason:     "Invalid query parameter: \"partition\" - Partitions are a Consul Enterprise feature",
		}
	}

	return nil
}

func (s *EnterpriseMetaHelperOSS) SerfMemberFillAuthzContext(m *serf.Member, ctx *acl.AuthorizerContext) {
	// no-op
}

func (s *EnterpriseMetaHelperOSS) AgentServiceFillAuthzContext(srv *api.AgentService, ctx *acl.AuthorizerContext) {
	// no-op
}
