package cslerr

var (
	BasicError = NewConsulError(0, "BasicError", "Fix it yourself")

	InvalidConfigEntryKind = NewConsulError(20, "Invalid config entry kind", "For a list of valid config entry kinds see the documentation at https://developer.hashicorp.com/consul/docs/connect/config-entries.")

	ACLNotFound         = NewConsulError(100, "ACL not found", "There is not an ACL with this ID. Check that you have the correct ACL ID or create an ACL with this ID if needed.")
	ACLRootDenied       = NewConsulError(101, "Cannot resolve root ACL", "Use a non-root ACL token")
	ACLDisabled         = NewConsulError(102, "ACL support disabled", "ACL changes are not permitted since they are disabled. Enable ACLs via the -enable-acl command line option or the enable_acl configuration option.")
	ACLPermissionDenied = NewConsulError(103, "Permission denied", "Check the ACL rules")
	ACLInvalidParent    = NewConsulError(104, "Invalid Parent", "Check the ACL rules")

	CAKeyGenerationFailed = NewConsulError(200, "Error generating ECDSA private key", "")
)
