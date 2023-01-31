package acl

import "hash"

// EnterpriseMeta interface
type EnterpriseMeta interface {
	ToEnterprisePolicyMeta() *EnterprisePolicyMeta
	EstimateSize() int
	AddToHash(_ hash.Hash, _ bool)
	PartitionOrDefault() string
	PartitionOrEmpty() string
	InDefaultPartition() bool
	NamespaceOrDefault() string
	NamespaceOrEmpty() string
	InDefaultNamespace() bool
	Merge(_ EnterpriseMeta)
	MergeNoWildcard(_ EnterpriseMeta)
	Normalize()
	NormalizePartition()
	NormalizeNamespace()
	Matches(_ EnterpriseMeta) bool
	IsSame(_ EnterpriseMeta) bool
	LessThan(_ EnterpriseMeta) bool
	WithWildcardNamespace() EnterpriseMeta
	UnsetPartition()
	OverridePartition(_ string)
	FillAuthzContext(_ *AuthorizerContext)
}
