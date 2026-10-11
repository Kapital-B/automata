package shared

// OperationError distinguishes use-case failures without transport status codes.
type OperationError struct {
	Kind    string
	Message string
	Cause   error
}

func (e *OperationError) Error() string { return e.Message }
func (e *OperationError) Unwrap() error { return e.Cause }
