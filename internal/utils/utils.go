package utils

import (
	"encoding/json"
	"fmt"
	"io"
)

// Secret holds a sensitive string. It implements fmt.Stringer, fmt.Formatter,
// json.Marshaler, encoding.TextMarshaler so that
// incidental printing or marshaling yields an obfuscated value.
// Call GetValue() to retrieve the real underlying string.
type Secret string

// New creates a Secret from a string.
func New(s string) Secret { return Secret(s) }

// GetValue returns the real underlying string.
func (s Secret) GetValue() string { return string(s) }

// IsZero reports whether the secret is empty.
func (s Secret) IsZero() bool { return len(s) == 0 }

// Equal compares the underlying value (constant-time comparison is not done
// here; use crypto/subtle if you need constant-time).
func (s Secret) Equal(other Secret) bool { return string(s) == string(other) }

// Error-safe obfuscation: keep a few characters and replace the rest with stars.
// Examples:
//
//	"a" -> "*"
//	"ab" -> "a*"
//	"abcd" -> "a**d"
//	"supersecret" -> "s********t"
func obfuscate(in string) string {
	n := len(in)

	// handle edge cases where len is too short
	if n == 0 {
		return ""
	}
	if n == 1 {
		return "*"
	}
	if n == 2 {
		return string(in[0]) + "*"
	}

	// keep first and last char, replace middle with stars (count = n-2)
	buf := make([]byte, 0, n)
	buf = append(buf, in[0])
	for i := 0; i < n-2; i++ {
		buf = append(buf, '*')
	}
	buf = append(buf, in[n-1])
	return string(buf)
}

// String implements fmt.Stringer and returns the obfuscated representation.
func (s Secret) String() string {
	return obfuscate(string(s))
}

// GoString implements fmt.GoStringer (used by %#v).
func (s Secret) GoString() string { return s.String() }

// Format gives full control over how fmt formats the type. We purposely
// present the obfuscated value for %s, %v, %q, etc.
// This prevents accidental leakage by typical fmt usage.
func (s Secret) Format(f fmt.State, c rune) {
	// We format the obfuscated value for common verbs.
	out := s.String()

	switch c {
	case 'v', 's':
		// if %+v or %#v requested we still print obfuscated
		io.WriteString(f, out)
	case 'q':
		// quoted string (keep quotes but content obfuscated)
		fmt.Fprintf(f, "%q", out)
	default:
		// fallback: just use fmt's default formatting on obfuscated string
		fmt.Fprintf(f, "%%!%c(%s)", c, out)
	}
}

// MarshalText returns the obfuscated value by default.
// Use s.GetValue() explicitly if you must produce the real text.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText stores the provided text as the real secret.
func (s *Secret) UnmarshalText(data []byte) error {
	*s = Secret(string(data))
	return nil
}

// MarshalJSON marshals the obfuscated value as a JSON string.
// If you need to send the real secret over the wire, explicitly call json.Marshal(secret.GetValue()).
func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}
