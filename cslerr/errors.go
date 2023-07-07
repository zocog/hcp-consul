package cslerr

var (
	BasicError = New(0, "BasicError", "Fix it yourself")

	ACLNotFound         = New(100, "ACL not found", "There is not an ACL with this ID. Check that you have the correct ACL ID or create an ACL with this ID if needed.")
	ACLRootDenied       = New(101, "Cannot resolve root ACL", "Use a non-root ACL token")
	ACLDisabled         = New(102, "ACL support disabled", "ACL changes are not permitted since they are disabled. Enable ACLs via the -enable-acl command line option or the enable_acl configuration option.")
	ACLPermissionDenied = New(103, "Permission denied", "Check the ACL rules")
	ACLInvalidParent    = New(104, "Invalid Parent", "Check the ACL rules")

	CAKeyGenerationFailed = New(200, "Error generating ECDSA private key", "")
)
