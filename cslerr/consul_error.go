package cslerr

import "fmt"

var (
	errorRegistry = make(map[int]*ConsulError)
)

// ConsulError is a custom error type that is used to represent errors
// originating in the Consul source code. It includes a code, message,
// and remedy.
// The code should be a fixed integer value that users can reference
type ConsulError struct {
	Code    int
	Message string
	Remedy  string

	inner []error
}

func NewConsulError(code int, message string, remedy string) *ConsulError {
	e := &ConsulError{
		Code:    code,
		Message: message,
		Remedy:  remedy,
	}

	// Register the error so that a collection of all Consul errors can be referenced later.
	// This is useful for generating documentation and for ensuring that error codes are unique.
	if _, ok := errorRegistry[code]; ok {
		panic(fmt.Sprintf("Consul error code %d is already registered", code))
	}
	errorRegistry[code] = e

	return e
}

func (e *ConsulError) Wrap(err error) *ConsulError {
	if e.inner == nil {
		e.inner = []error{err}
		return e
	}

	e.inner = append(e.inner, err)
	return e
}

func (e *ConsulError) AddRemedy(remedy string) *ConsulError {
	e.Remedy = fmt.Sprintf("%s\n%s", e.Remedy, remedy)
	return e
}

func (e *ConsulError) Error() string {
	if len(e.inner) > 0 {
		return fmt.Sprintf("CSL%04d: %s\n%s\n%s", e.Code, e.Message, e.Remedy, e.inner)
	}

	return fmt.Sprintf("CSL%04d: %s\n%s", e.Code, e.Message, e.Remedy)
}
