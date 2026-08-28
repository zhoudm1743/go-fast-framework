package utils

import (
	"strconv"
	"strings"
)

// VersionUtil 版本比较工具集。
var VersionUtil = versionUtil{}

type versionUtil struct{}

func (r versionUtil) Compare(a, b string) int {
	av := parseVersion(a)
	bv := parseVersion(b)
	maxLen := len(av)
	if len(bv) > maxLen {
		maxLen = len(bv)
	}
	for i := 0; i < maxLen; i++ {
		ai, bi := 0, 0
		if i < len(av) {
			ai = av[i]
		}
		if i < len(bv) {
			bi = bv[i]
		}
		if ai > bi {
			return 1
		}
		if ai < bi {
			return -1
		}
	}
	return 0
}

func (r versionUtil) GTE(a, b string) bool { return r.Compare(a, b) >= 0 }
func (r versionUtil) GT(a, b string) bool  { return r.Compare(a, b) > 0 }
func (r versionUtil) LTE(a, b string) bool { return r.Compare(a, b) <= 0 }
func (r versionUtil) LT(a, b string) bool  { return r.Compare(a, b) < 0 }
func (r versionUtil) EQ(a, b string) bool  { return r.Compare(a, b) == 0 }

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.SplitN(p, "-", 2)[0]
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}
