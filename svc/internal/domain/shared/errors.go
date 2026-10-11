package shared

import "errors"

// ErrNotFound reports absence only for ports whose contract uses an error.
var ErrNotFound = errors.New("record not found")
