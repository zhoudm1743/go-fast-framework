package utils

import (
	"net/url"
	"path"
	"strings"
)

// UrlUtil URL 工具集（链式）。
var UrlUtil = urlUtil{}

type urlUtil struct{}

type URL struct {
	u *url.URL
}

func (r urlUtil) Of(raw string) *URL {
	u, err := url.Parse(raw)
	if err != nil {
		u = &url.URL{}
	}
	return &URL{u: u}
}

func (u *URL) Join(segments ...string) *URL {
	p := u.u.Path
	for _, s := range segments {
		p = path.Join(p, s)
	}
	u.u.Path = p
	return u
}

func (u *URL) WithQuery(params map[string]string) *URL {
	q := u.u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.u.RawQuery = q.Encode()
	return u
}

func (u *URL) SetQuery(key, value string) *URL {
	q := u.u.Query()
	q.Set(key, value)
	u.u.RawQuery = q.Encode()
	return u
}

func (u *URL) StripQuery() *URL {
	u.u.RawQuery = ""
	return u
}

func (u *URL) Scheme(s string) *URL {
	u.u.Scheme = s
	return u
}

func (u *URL) Host(h string) *URL {
	u.u.Host = h
	return u
}

func (u *URL) String() string {
	return u.u.String()
}

func (u *URL) Path() string {
	return u.u.Path
}

func (u *URL) Hostname() string {
	return u.u.Hostname()
}

func (u *URL) QueryToMap() map[string]string {
	out := make(map[string]string)
	for k, vals := range u.u.Query() {
		if len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

func (r urlUtil) Encode(s string) string {
	return url.QueryEscape(s)
}

func (r urlUtil) Decode(s string) (string, error) {
	return url.QueryUnescape(s)
}

func (r urlUtil) MapToQuery(m map[string]string) string {
	q := url.Values{}
	for k, v := range m {
		q.Set(k, v)
	}
	return q.Encode()
}

func (r urlUtil) IsAbsolute(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}
