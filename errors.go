package kulynflags

import (
	"fmt"
	"strings"
)

// ErrorCode classifies a RegistryError. Use errors.Is with the constants below.
type ErrorCode string

// Error implements the error interface. It returns the code itself, so that an
// ErrorCode can be used as a sentinel for errors.Is.
func (c ErrorCode) Error() string {
	return string(c)
}

// Error codes returned by NewRegistry and its options.
const (
	// ErrInvalidKey means a flag key does not match the key format.
	ErrInvalidKey ErrorCode = "INVALID_KEY"
	// ErrDuplicateKey means two flags share the same key.
	ErrDuplicateKey ErrorCode = "DUPLICATE_KEY"
	// ErrMissingOwner means a flag has an empty or blank owner.
	ErrMissingOwner ErrorCode = "MISSING_OWNER"
	// ErrInvalidDates means createdAt is missing, or expiresAt is not after createdAt.
	ErrInvalidDates ErrorCode = "INVALID_DATES"
	// ErrMissingExpiry means a flag of a kind with a maximum lifetime has no expiry.
	ErrMissingExpiry ErrorCode = "MISSING_EXPIRY"
	// ErrInvalidKind means a flag or an option uses an unknown kind.
	ErrInvalidKind ErrorCode = "INVALID_KIND"
	// ErrInvalidOption means an option has an invalid value, such as a negative day count.
	ErrInvalidOption ErrorCode = "INVALID_OPTION"
)

// RegistryError is returned by NewRegistry and its options when the register is invalid.
// Code classifies the error. Key names the offending flag when it is known.
type RegistryError struct {
	Code    ErrorCode
	Key     string
	Message string
}

// Error implements the error interface.
func (e *RegistryError) Error() string {
	if e.Message == "" {
		return string(e.Code)
	}
	return e.Message
}

// Unwrap returns the error code, so that errors.Is(err, ErrInvalidKey) works.
func (e *RegistryError) Unwrap() error {
	return e.Code
}

// HygieneError is returned by AssertHygiene. Findings holds every finding that matched.
type HygieneError struct {
	Findings []Finding
}

// Error implements the error interface and lists all matching findings.
func (e *HygieneError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "flag hygiene check failed with %d finding(s):", len(e.Findings))
	for _, finding := range e.Findings {
		fmt.Fprintf(&b, "\n  - [%s] %s", finding.Code, finding.Message)
	}
	return b.String()
}
