package message

import (
	"errors"
	"mime"
	"strings"

	"github.com/emersion/go-message/textproto"
)

// MalformedHeaderError is returned alongside recovered values when a header
// field was malformed but could be partially recovered (e.g. duplicate
// parameters). The accompanying return values are valid and safe to use.
// The Err field holds the original underlying parse error.
type MalformedHeaderError struct {
	Err error
}

func (e *MalformedHeaderError) Error() string { return e.Err.Error() }
func (e *MalformedHeaderError) Unwrap() error { return e.Err }

// IsMalformedHeader reports whether err signals a header that was malformed
// but recovered. When true, the other return values from ContentType or
// ContentDisposition are valid and should be used.
func IsMalformedHeader(err error) bool {
	return errors.As(err, new(*MalformedHeaderError))
}

// deduplicateContentTypeParams returns a copy of s with duplicate parameter
// names removed (first occurrence wins). It handles quoted-string values that
// may contain semicolons or backslash-escaped characters.
//
// Two-pass design: the first pass detects whether any duplicate exists using a
// stack-allocated array and strings.EqualFold (no heap allocations). The second
// pass only runs — and only allocates — when a duplicate is actually found.
func deduplicateContentTypeParams(s string) string {
	idx := strings.IndexByte(s, ';')
	if idx < 0 {
		return s
	}

	// First pass: scan param names to detect any duplicate.
	// The fixed-size array covers the vast majority of real Content-Type headers;
	// if more than 8 params are present we conservatively trigger a rebuild.
	var names [8]string
	n := 0
	hasDup := false

	rest := s[idx:]
	for len(rest) > 0 && !hasDup {
		if rest[0] != ';' {
			rest = rest[1:]
			continue
		}
		rest = rest[1:]
		rest = strings.TrimLeft(rest, " \t\r\n")

		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx < 0 {
			break
		}
		if semiBeforeEq := strings.IndexByte(rest[:eqIdx], ';'); semiBeforeEq >= 0 {
			rest = rest[semiBeforeEq:]
			continue
		}

		name := strings.TrimRight(rest[:eqIdx], " \t")
		rest = rest[eqIdx+1:]

		// Check for a duplicate before scanning past the value — if we find one
		// we break immediately and skip the value-scanning work entirely.
		if n >= len(names) {
			hasDup = true // more params than our array — rebuild conservatively
			break
		}
		for i := range n {
			if strings.EqualFold(names[i], name) {
				hasDup = true
				break
			}
		}
		if hasDup {
			break
		}
		names[n] = name
		n++

		// Skip past the value to advance to the next param.
		if len(rest) > 0 && rest[0] == '"' {
			end := 1
			for end < len(rest) {
				if rest[end] == '\\' {
					end += 2
				} else if rest[end] == '"' {
					end++
					break
				} else {
					end++
				}
			}
			rest = rest[end:]
			rest = strings.TrimLeft(rest, " \t\r\n")
		} else if semi := strings.IndexByte(rest, ';'); semi >= 0 {
			rest = rest[semi:]
		} else {
			rest = ""
		}
	}

	if !hasDup {
		return s // no duplicates — return the original string unchanged
	}

	// Second pass: rebuild the string with duplicates removed.
	seen := make(map[string]bool)
	var result strings.Builder
	result.WriteString(s[:idx])

	rest = s[idx:]
	for len(rest) > 0 {
		if rest[0] != ';' {
			rest = rest[1:]
			continue
		}
		rest = rest[1:]
		rest = strings.TrimLeft(rest, " \t\r\n")

		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx < 0 {
			break
		}
		if semiBeforeEq := strings.IndexByte(rest[:eqIdx], ';'); semiBeforeEq >= 0 {
			rest = rest[semiBeforeEq:]
			continue
		}

		name := strings.TrimRight(rest[:eqIdx], " \t")
		rest = rest[eqIdx+1:]

		var value string
		if len(rest) > 0 && rest[0] == '"' {
			end := 1
			for end < len(rest) {
				if rest[end] == '\\' {
					end += 2
				} else if rest[end] == '"' {
					end++
					break
				} else {
					end++
				}
			}
			value = rest[:end]
			rest = rest[end:]
			rest = strings.TrimLeft(rest, " \t\r\n")
		} else if semi := strings.IndexByte(rest, ';'); semi >= 0 {
			value = strings.TrimRight(rest[:semi], " \t\r\n")
			rest = rest[semi:]
		} else {
			value = strings.TrimRight(rest, " \t\r\n")
			rest = ""
		}

		lower := strings.ToLower(name)
		if name != "" && !seen[lower] {
			seen[lower] = true
			result.WriteString("; ")
			result.WriteString(name)
			result.WriteByte('=')
			result.WriteString(value)
		}
	}

	return result.String()
}

func parseHeaderWithParams(s string) (f string, params map[string]string, err error) {
	f, params, err = mime.ParseMediaType(s)
	if err != nil {
		// Try recovery by removing duplicate parameter names
		deduped := deduplicateContentTypeParams(s)
		var recoveredF string
		var recoveredParams map[string]string
		recoveredF, recoveredParams, _ = mime.ParseMediaType(deduped)
		if recoveredParams != nil {
			// Wrap the original error so callers can distinguish a recovered
			// malformed header (where the return values are valid) from a
			// genuinely unparseable one (where params is nil).
			f = recoveredF
			params = recoveredParams
			err = &MalformedHeaderError{Err: err}
		} else {
			return s, nil, err
		}
	}
	for k, v := range params {
		params[k], _ = decodeHeader(v)
	}
	return
}

func formatHeaderWithParams(f string, params map[string]string) string {
	encParams := make(map[string]string)
	for k, v := range params {
		encParams[k] = encodeHeader(v)
	}
	return mime.FormatMediaType(f, encParams)
}

// HeaderFields iterates over header fields.
type HeaderFields interface {
	textproto.HeaderFields

	// Text parses the value of the current field as plaintext. The field
	// charset is decoded to UTF-8. If the header field's charset is unknown,
	// the raw field value is returned and the error verifies IsUnknownCharset.
	Text() (string, error)

	headerFields()
}

type headerFields struct {
	textproto.HeaderFields
}

func (*headerFields) headerFields() {}

func (hf *headerFields) Text() (string, error) {
	return decodeHeader(hf.Value())
}

// A Header represents the key-value pairs in a message header.
type Header struct {
	textproto.Header
}

// HeaderFromMap creates a header from a map of header fields.
//
// This function is provided for interoperability with the standard library.
// If possible, ReadHeader should be used instead to avoid loosing information.
// The map representation looses the ordering of the fields, the capitalization
// of the header keys, and the whitespace of the original header.
func HeaderFromMap(m map[string][]string) Header {
	return Header{textproto.HeaderFromMap(m)}
}

// ContentType parses the Content-Type header field.
//
// If no Content-Type is specified, it returns "text/plain".
func (h *Header) ContentType() (t string, params map[string]string, err error) {
	v := h.Get("Content-Type")
	if v == "" {
		return "text/plain", nil, nil
	}
	return parseHeaderWithParams(v)
}

// SetContentType formats the Content-Type header field.
func (h *Header) SetContentType(t string, params map[string]string) {
	h.Set("Content-Type", formatHeaderWithParams(t, params))
}

// ContentDisposition parses the Content-Disposition header field, as defined in
// RFC 2183.
func (h *Header) ContentDisposition() (disp string, params map[string]string, err error) {
	return parseHeaderWithParams(h.Get("Content-Disposition"))
}

// SetContentDisposition formats the Content-Disposition header field, as
// defined in RFC 2183.
func (h *Header) SetContentDisposition(disp string, params map[string]string) {
	h.Set("Content-Disposition", formatHeaderWithParams(disp, params))
}

// Text parses a plaintext header field. The field charset is automatically
// decoded to UTF-8. If the header field's charset is unknown, the raw field
// value is returned and the error verifies IsUnknownCharset.
func (h *Header) Text(k string) (string, error) {
	return decodeHeader(h.Get(k))
}

// SetText sets a plaintext header field.
func (h *Header) SetText(k, v string) {
	h.Set(k, encodeHeader(v))
}

// Copy creates a stand-alone copy of the header.
func (h *Header) Copy() Header {
	return Header{h.Header.Copy()}
}

// Fields iterates over all the header fields.
//
// The header may not be mutated while iterating, except using HeaderFields.Del.
func (h *Header) Fields() HeaderFields {
	return &headerFields{h.Header.Fields()}
}

// FieldsByKey iterates over all fields having the specified key.
//
// The header may not be mutated while iterating, except using HeaderFields.Del.
func (h *Header) FieldsByKey(k string) HeaderFields {
	return &headerFields{h.Header.FieldsByKey(k)}
}
