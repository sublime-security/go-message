// Package charset provides functions to decode and encode charsets.
//
// It imports all supported charsets, which adds about 1MiB to binaries size.
// Importing the package automatically sets message.CharsetReader.
package charset

import (
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-message"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/unicode"
)

// Quirks table for charsets not handled by ianaindex
//
// A nil entry disables the charset.
//
// For aliases, see
// https://www.iana.org/assignments/character-sets/character-sets.xhtml
var charsets = map[string]encoding.Encoding{
	"ansi_x3.110-1983": charmap.ISO8859_1, // see RFC 1345 page 62, mostly superset of ISO 8859-1
	"x-utf_8j":         unicode.UTF8,      // alias for UTF-8, see https://icu4c-demos.unicode.org/icu-bin/convexp?s=ALL
}

func init() {
	message.CharsetReader = Reader
	message.CharsetWriter = Writer
}

// charsetEncoding returns the appropriate encoding.Encoding for the provided charset
func charsetEncoding(charset string) (encoding.Encoding, error) {
	var err error
	enc, ok := charsets[strings.ToLower(charset)]
	if ok && enc == nil {
		return nil, fmt.Errorf("charset %q: charset is disabled", charset)
	} else if !ok {
		enc, err = ianaindex.MIME.Encoding(charset)
	}
	if enc == nil {
		enc, err = ianaindex.MIME.Encoding("cs" + charset)
	}
	if enc == nil {
		enc, err = htmlindex.Get(charset)
	}
	if err != nil {
		return nil, fmt.Errorf("charset %q: %v", charset, err)
	}
	// See https://github.com/golang/go/issues/19421
	if enc == nil {
		return nil, fmt.Errorf("charset %q: unsupported charset", charset)
	}
	return enc, nil
}

// Reader returns an io.Reader that converts the provided charset to UTF-8.
func Reader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := charsetEncoding(charset)
	if err != nil {
		return input, err
	}
	return enc.NewDecoder().Reader(input), nil
}

// Writer returns an io.Writer than converts the UTF-8 output from writer to the provided charset.
func Writer(charset string, writer io.Writer) (io.Writer, error) {
	enc, err := charsetEncoding(charset)
	if err != nil {
		return writer, err
	}
	return enc.NewEncoder().Writer(writer), nil
}

// RegisterEncoding registers an encoding. This is intended to be called from
// the init function in packages that want to support additional charsets.
func RegisterEncoding(name string, enc encoding.Encoding) {
	charsets[name] = enc
}
