package utils

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	"unicode/utf8"
)

// EncodingUtil 编解码工具集。
var EncodingUtil = encodingUtil{}

type encodingUtil struct{}

func (r encodingUtil) Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func (r encodingUtil) Base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func (r encodingUtil) Base64URLEncode(data []byte) string {
	return base64.URLEncoding.EncodeToString(data)
}

func (r encodingUtil) Base64URLDecode(s string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(s)
}

func (r encodingUtil) HexEncode(data []byte) string {
	return hex.EncodeToString(data)
}

func (r encodingUtil) HexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

func (r encodingUtil) Gzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r encodingUtil) Gunzip(data []byte) ([]byte, error) {
	rdr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer rdr.Close()
	return io.ReadAll(rdr)
}

func (r encodingUtil) BOMStrip(s string) string {
	if strings.HasPrefix(s, "\ufeff") {
		return strings.TrimPrefix(s, "\ufeff")
	}
	if len(s) >= 3 && s[0] == 0xef && s[1] == 0xbb && s[2] == 0xbf {
		return s[3:]
	}
	if !utf8.ValidString(s) {
		return s
	}
	return s
}
