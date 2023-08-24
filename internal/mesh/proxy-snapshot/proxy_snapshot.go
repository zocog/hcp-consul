package proxysnapshot

// ProxySnapshot is an abstraction that allows interchangeability between
// Catalog V1 ConfigSnapshot and Catalog V2 ProxyState.
type ProxySnapshot interface {
	AllowEmptyListeners() bool
	AllowEmptyRoutes() bool
	AllowEmptyClusters() bool
	//Authorize(authz acl.Authorizer) error
	LoggerName() string
}

// CancelFunc is a type for a returned function that can be called to cancel a
// watch.
type CancelFunc func()

// SessionTerminatedChan is a channel that will be closed to notify session-
// holders that a session has been terminated.
type SessionTerminatedChan <-chan struct{}

type Session interface {
	// End the session.
	//
	// This MUST be called when the session-holder is done (e.g. the gRPC stream
	// is closed).
	End()

	// Terminated is a channel that is closed when the session is terminated.
	//
	// The session-holder MUST receive on it and exit (e.g. close the gRPC stream)
	// when it is closed.
	Terminated() SessionTerminatedChan
}
