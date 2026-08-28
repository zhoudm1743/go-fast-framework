package utils

import (
	"strconv"
	"strings"
	"time"
)

// IdCardUtil 身份证工具集。
var IdCardUtil = idCardUtil{}

type idCardUtil struct{}

type IDCardInfo struct {
	Birthday time.Time
	Gender   string
	Region   string
	Age      int
}

var idCardWeights = []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
var idCardCheck = []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}

func (r idCardUtil) Valid(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 18 {
		return false
	}
	for i := 0; i < 17; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	sum := 0
	for i := 0; i < 17; i++ {
		n, _ := strconv.Atoi(string(s[i]))
		sum += n * idCardWeights[i]
	}
	return idCardCheck[sum%11] == s[17]
}

func (r idCardUtil) Parse(s string) (*IDCardInfo, error) {
	if !r.Valid(s) {
		return nil, ErrInvalidIDCard
	}
	s = strings.ToUpper(s)
	birthStr := s[6:14]
	birth, err := time.ParseInLocation("20060102", birthStr, defaultLoc())
	if err != nil {
		return nil, err
	}
	genderCode, _ := strconv.Atoi(string(s[16]))
	gender := "女"
	if genderCode%2 == 1 {
		gender = "男"
	}
	age := calcAge(birth)
	return &IDCardInfo{
		Birthday: birth,
		Gender:   gender,
		Region:   s[:6],
		Age:      age,
	}, nil
}

func (r idCardUtil) Age(s string) int {
	if !r.Valid(s) {
		return 0
	}
	birthStr := strings.ToUpper(s)[6:14]
	birth, err := time.ParseInLocation("20060102", birthStr, defaultLoc())
	if err != nil {
		return 0
	}
	return calcAge(birth)
}

func calcAge(birth time.Time) int {
	now := time.Now()
	age := now.Year() - birth.Year()
	if now.YearDay() < birth.YearDay() {
		age--
	}
	return age
}

func (r idCardUtil) IsAdult(s string) bool {
	return r.Age(s) >= 18
}

// ErrInvalidIDCard 无效身份证号。
var ErrInvalidIDCard = errInvalidIDCard{}

type errInvalidIDCard struct{}

func (errInvalidIDCard) Error() string { return "invalid id card" }
