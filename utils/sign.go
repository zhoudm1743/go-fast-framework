package utils

import (
	"sort"
	"strings"
)

// SignUtil 签名工具集。
var SignUtil = signUtil{}

type signUtil struct{}

func (r signUtil) SortQuery(m map[string]string, skipEmpty bool) string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if skipEmpty && v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, "&")
}

func (r signUtil) MD5(data string) string {
	return ToolsUtil.Md5(data)
}

func (r signUtil) SHA256(data string) string {
	return HashUtil.Sha256(data)
}

func (r signUtil) HMAC(data, secret string) string {
	return HashUtil.HmacSha256(data, secret)
}

func (r signUtil) SignMD5(m map[string]string, secret string, skipEmpty bool) string {
	return r.MD5(r.SortQuery(m, skipEmpty) + secret)
}

func (r signUtil) SignSHA256(m map[string]string, secret string, skipEmpty bool) string {
	return r.SHA256(r.SortQuery(m, skipEmpty) + secret)
}

func (r signUtil) SignHMAC(m map[string]string, secret string, skipEmpty bool) string {
	return r.HMAC(r.SortQuery(m, skipEmpty), secret)
}
