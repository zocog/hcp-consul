// x-build !consulent

package acl

import "hash"

var emptyEnterpriseMeta = EnterpriseMetaOSS{}

// EnterpriseMetaOSS stub
type EnterpriseMetaOSS struct{}

func (m *EnterpriseMetaOSS) ToEnterprisePolicyMeta() *EnterprisePolicyMeta {
	return nil
}

func DefaultEnterpriseMeta() *EnterpriseMetaOSS {
	return &EnterpriseMetaOSS{}
}

func WildcardEnterpriseMeta() *EnterpriseMetaOSS {
	return &EnterpriseMetaOSS{}
}

func (m *EnterpriseMetaOSS) EstimateSize() int {
	return 0
}

func (m *EnterpriseMetaOSS) AddToHash(_ hash.Hash, _ bool) {
	// do nothing
}

func (m *EnterpriseMetaOSS) PartitionOrDefault() string {
	return "default"
}

func EqualPartitions(_, _ string) bool {
	return true
}

func IsDefaultPartition(partition string) bool {
	return true
}

func PartitionOrDefault(_ string) string {
	return "default"
}

func (m *EnterpriseMetaOSS) PartitionOrEmpty() string {
	return ""
}

func (m *EnterpriseMetaOSS) InDefaultPartition() bool {
	return true
}

func (m *EnterpriseMetaOSS) NamespaceOrDefault() string {
	return DefaultNamespaceName
}

func EqualNamespaces(_, _ string) bool {
	return true
}

func NamespaceOrDefault(_ string) string {
	return DefaultNamespaceName
}

func (m *EnterpriseMetaOSS) NamespaceOrEmpty() string {
	return ""
}

func (m *EnterpriseMetaOSS) InDefaultNamespace() bool {
	return true
}

func (m *EnterpriseMetaOSS) Merge(_ EnterpriseMeta) {
	// do nothing
}

func (m *EnterpriseMetaOSS) MergeNoWildcard(_ EnterpriseMeta) {
	// do nothing
}

func (_ *EnterpriseMetaOSS) Normalize()          {}
func (_ *EnterpriseMetaOSS) NormalizePartition() {}
func (_ *EnterpriseMetaOSS) NormalizeNamespace() {}

func (m *EnterpriseMetaOSS) Matches(_ EnterpriseMeta) bool {
	return true
}

func (m *EnterpriseMetaOSS) IsSame(_ EnterpriseMeta) bool {
	return true
}

func (m *EnterpriseMetaOSS) LessThan(_ EnterpriseMeta) bool {
	return false
}

func (m *EnterpriseMetaOSS) WithWildcardNamespace() EnterpriseMeta {
	return &emptyEnterpriseMeta
}

func (m *EnterpriseMetaOSS) UnsetPartition() {
	// do nothing
}

func (m *EnterpriseMetaOSS) OverridePartition(_ string) {
	// do nothing
}

func NewEnterpriseMetaWithPartition(_, _ string) EnterpriseMetaOSS {
	return emptyEnterpriseMeta
}

// FillAuthzContext stub
func (_ *EnterpriseMetaOSS) FillAuthzContext(_ *AuthorizerContext) {}

func NormalizeNamespace(_ string) string {
	return ""
}
