package utils

import (
	"bytes"
	"io"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// CharsetUtil 字符集转换工具集。
var CharsetUtil = charsetUtil{}

type charsetUtil struct{}

func (r charsetUtil) GBKToUTF8(data []byte) ([]byte, error) {
	return io.ReadAll(transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder()))
}

func (r charsetUtil) UTF8ToGBK(data []byte) ([]byte, error) {
	return io.ReadAll(transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewEncoder()))
}

func (r charsetUtil) GB18030ToUTF8(data []byte) ([]byte, error) {
	return io.ReadAll(transform.NewReader(bytes.NewReader(data), simplifiedchinese.GB18030.NewDecoder()))
}
