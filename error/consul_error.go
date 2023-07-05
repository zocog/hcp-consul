package error

import "fmt"

type ConsulError struct {
	Code    int
	Message string
	Remedy  string
}

func NewConsulError(code int, message string, remedy string) *ConsulError {
	return &ConsulError{
		Code:    code,
		Message: message,
		Remedy:  remedy,
	}
}

func (e *ConsulError) Error() string {
	return fmt.Sprintf("CSL%04d: %s %s", e.Code, e.Message, e.Remedy)
}
