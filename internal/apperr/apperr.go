// Package apperr defines coded errors that cross the Wails boundary. The
// frontend receives Error() as a JSON document and decodes it.
package apperr

import (
	"encoding/json"
	"errors"

	"anansi/internal/database"
)

// Error codes understood by the frontend.
const (
	CodeValidation           = "validation"
	CodeNotFound             = "not_found"
	CodeNotConnected         = "not_connected"
	CodePasswordRequired     = "password_required"
	CodeKeyringUnavailable   = "keyring_unavailable"
	CodeDatabase             = "database"
	CodeConfirmationRequired = "confirmation_required"
	CodeReadOnly             = "read_only"
	CodeBusy                 = "busy"
	CodeCancelled            = "cancelled"
	CodeInternal             = "internal"
	CodeSSHSecretRequired    = "ssh_secret_required"
	CodeSSHHostUnknown       = "ssh_host_unknown"
)

// Error is a structured application error.
type Error struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	DB      *database.DBError `json:"db,omitempty"`
	Details any               `json:"details,omitempty"`
}

func (e *Error) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return e.Message
	}
	return string(b)
}

// New creates an error.
func New(code, msg string) *Error { return &Error{Code: code, Message: msg} }

// FromDB wraps a decoded database error.
func FromDB(de *database.DBError) *Error {
	code := CodeDatabase
	if de.Category == "cancelled" {
		code = CodeCancelled
	}
	if de.Category == "validation" {
		code = CodeValidation
	}
	return &Error{Code: code, Message: de.Message, DB: de}
}

// Wrap converts any error into *Error, preserving existing codes.
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	var de *database.DBError
	if errors.As(err, &de) {
		return FromDB(de)
	}
	return &Error{Code: CodeInternal, Message: err.Error()}
}
