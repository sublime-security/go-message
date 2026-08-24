package message

import (
	"reflect"
	"testing"
)

func TestHeader(t *testing.T) {
	mediaType := "text/plain"
	mediaParams := map[string]string{"charset": "utf-8"}
	desc := "Plan de complémentarité de l'Homme"
	disp := "attachment"
	dispParams := map[string]string{"filename": "complémentarité.txt"}

	var h Header
	h.SetContentType(mediaType, mediaParams)
	h.SetText("Content-Description", desc)
	h.SetContentDisposition(disp, dispParams)

	if gotMediaType, gotParams, err := h.ContentType(); err != nil {
		t.Error("Expected no error when parsing content type, but got:", err)
	} else if gotMediaType != mediaType {
		t.Errorf("Expected media type %q but got %q", mediaType, gotMediaType)
	} else if !reflect.DeepEqual(gotParams, mediaParams) {
		t.Errorf("Expected media params %v but got %v", mediaParams, gotParams)
	}

	if gotDesc, err := h.Text("Content-Description"); err != nil {
		t.Error("Expected no error when parsing content description, but got:", err)
	} else if gotDesc != desc {
		t.Errorf("Expected content description %q but got %q", desc, gotDesc)
	}

	if gotDisp, gotParams, err := h.ContentDisposition(); err != nil {
		t.Error("Expected no error when parsing content disposition, but got:", err)
	} else if gotDisp != disp {
		t.Errorf("Expected disposition %q but got %q", disp, gotDisp)
	} else if !reflect.DeepEqual(gotParams, dispParams) {
		t.Errorf("Expected disposition params %v but got %v", dispParams, gotParams)
	}
}

func TestEmptyContentType(t *testing.T) {
	var h Header

	mediaType := "text/plain"
	if gotMediaType, _, err := h.ContentType(); err != nil {
		t.Error("Expected no error when parsing empty content type, but got:", err)
	} else if gotMediaType != mediaType {
		t.Errorf("Expected media type %q but got %q", mediaType, gotMediaType)
	}
}

func TestKnownCharset(t *testing.T) {
	var h Header

	h.Set("Subject", "=?ISO-8859-1?B?SWYgeW91IGNhbiByZWFkIHRoaXMgeW8=?=")

	fields := h.Fields()
	if !fields.Next() {
		t.Error("Expected to be able to advance to first header item")
	}

	_, err := fields.Text()
	if err != nil {
		t.Error("Expected no error when decoding header key of known charset, but got: ", err)
	}
}

func TestDeduplicateContentTypeParams(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no params",
			input: "multipart/mixed",
			want:  "multipart/mixed",
		},
		{
			name:  "no duplicates",
			input: `multipart/mixed; boundary=abc`,
			want:  `multipart/mixed; boundary=abc`,
		},
		{
			name:  "duplicate boundary keeps first",
			input: `multipart/mixed; boundary=abc; boundary=xyz`,
			want:  `multipart/mixed; boundary=abc`,
		},
		{
			name:  "duplicate charset keeps first",
			input: `text/html; charset=utf-8; charset=us-ascii`,
			want:  `text/html; charset=utf-8`,
		},
		{
			name:  "quoted value with semicolon",
			input: `multipart/mixed; boundary="ab;cd"; boundary=xyz`,
			want:  `multipart/mixed; boundary="ab;cd"`,
		},
		{
			name:  "case-insensitive param names",
			input: `text/plain; charset=utf-8; CHARSET=us-ascii`,
			want:  `text/plain; charset=utf-8`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deduplicateContentTypeParams(tc.input)
			if got != tc.want {
				t.Errorf("deduplicateContentTypeParams(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestContentTypeDuplicateParamRecovery(t *testing.T) {
	t.Run("normal header returns no error", func(t *testing.T) {
		var h Header
		h.Set("Content-Type", `multipart/mixed; boundary=abc`)

		mediaType, params, err := h.ContentType()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if mediaType != "multipart/mixed" {
			t.Errorf("expected media type %q, got %q", "multipart/mixed", mediaType)
		}
		if params["boundary"] != "abc" {
			t.Errorf("expected boundary %q, got %q", "abc", params["boundary"])
		}
	})

	t.Run("duplicate param is recovered with MalformedHeader error", func(t *testing.T) {
		var h Header
		h.Set("Content-Type", `multipart/mixed; boundary=abc; boundary=xyz`)

		mediaType, params, err := h.ContentType()
		if !IsMalformedHeader(err) {
			t.Errorf("expected IsMalformedHeader error, got %v", err)
		}
		if mediaType != "multipart/mixed" {
			t.Errorf("expected media type %q, got %q", "multipart/mixed", mediaType)
		}
		if params["boundary"] != "abc" {
			t.Errorf("expected boundary %q, got %q", "abc", params["boundary"])
		}
	})
}

func TestUnknownCharset(t *testing.T) {
	var h Header

	h.Set("Subject", "=?INVALIDCHARSET?B?dSB1bmRlcnN0YW5kIHRoZSBleGFtcGxlLg==?=")

	fields := h.Fields()
	if !fields.Next() {
		t.Error("Expected to be able to advance to first header item")
	}

	_, err := fields.Text()
	if err == nil {
		t.Error("Expected error decoding header key of unknown charset")
	}
	if !IsUnknownCharset(err) {
		t.Error("Expected error to verify IsUnknownCharset")
	}
}
