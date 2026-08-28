package utils

import (
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"strconv"
)

// CheckUtil 格式校验工具集。
var CheckUtil = checkUtil{}

type checkUtil struct{}

var (
	mobileRe     = regexp.MustCompile(`^1[3-9]\d{9}$`)
	landlineRe   = regexp.MustCompile(`^0\d{2,3}-?\d{7,8}$`)
	emailRe      = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	numericRe    = regexp.MustCompile(`^\d+$`)
	alphaRe      = regexp.MustCompile(`^[a-zA-Z]+$`)
	creditCodeRe = regexp.MustCompile(`^[0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10}$`)
	plateRe      = regexp.MustCompile(`^[\p{Han}][A-Z][A-HJ-NP-Z0-9]{5,6}$`)
	chineseRe    = regexp.MustCompile(`^[\p{Han}]+$`)
	chineseNameRe = regexp.MustCompile(`^[\p{Han}·]{2,20}$`)
	macRe        = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
	hexRe        = regexp.MustCompile(`^[0-9A-Fa-f]+$`)
	base64Re     = regexp.MustCompile(`^[A-Za-z0-9+/=]+$`)
)

func (r checkUtil) IsMobile(s string) bool       { return mobileRe.MatchString(s) }
func (r checkUtil) IsLandline(s string) bool     { return landlineRe.MatchString(s) }
func (r checkUtil) IsEmail(s string) bool        { return emailRe.MatchString(s) }
func (r checkUtil) IsNumeric(s string) bool      { return numericRe.MatchString(s) }
func (r checkUtil) IsAlpha(s string) bool        { return alphaRe.MatchString(s) }
func (r checkUtil) IsChinese(s string) bool      { return chineseRe.MatchString(s) }
func (r checkUtil) IsChineseName(s string) bool   { return chineseNameRe.MatchString(s) }
func (r checkUtil) IsHex(s string) bool          { return hexRe.MatchString(s) }
func (r checkUtil) IsBase64(s string) bool       { return base64Re.MatchString(s) }
func (r checkUtil) IsMac(s string) bool          { return macRe.MatchString(s) }
func (r checkUtil) IsCreditCode(s string) bool   { return creditCodeRe.MatchString(s) }
func (r checkUtil) IsPlate(s string) bool         { return plateRe.MatchString(s) }

func (r checkUtil) IsURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func (r checkUtil) IsJSON(s string) bool {
	return json.Valid([]byte(s))
}

func (r checkUtil) IsIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func (r checkUtil) IsIDCard(s string) bool {
	return IdCardUtil.Valid(s)
}

func (r checkUtil) IsBankCard(s string) bool {
	if len(s) < 13 || len(s) > 19 {
		return false
	}
	sum := 0
	alt := false
	for i := len(s) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(string(s[i]))
		if err != nil {
			return false
		}
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}
