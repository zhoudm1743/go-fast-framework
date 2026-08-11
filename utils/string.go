package utils

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var StringUtil = stringUtil{}

type stringUtil struct{}

func (r stringUtil) Contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func (r stringUtil) HasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func (r stringUtil) HasSuffix(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

func (r stringUtil) RandString(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// unsafeString converts a byte slice to a string without memory allocation.
func (r stringUtil) UnsafeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

type Str struct {
	value string
}

type excerptOption struct {
	Omission string
	Radius   int
}

// Of creates a new String instance with the given value.
func (r stringUtil) Of(value string) *Str {
	return &Str{value: value}
}

// After returns a new String instance with the substring after the first occurrence of the specified search string.
func (s *Str) After(search string) *Str {
	if search == "" {
		return s
	}
	index := strings.Index(s.value, search)
	if index != -1 {
		s.value = s.value[index+len(search):]
	}
	return s
}

// AfterLast returns the String instance with the substring after the last occurrence of the specified search string.
func (s *Str) AfterLast(search string) *Str {
	index := strings.LastIndex(s.value, search)
	if index != -1 {
		s.value = s.value[index+len(search):]
	}

	return s
}

// Append appends one or more strings to the current string.
func (s *Str) Append(values ...string) *Str {
	s.value += strings.Join(values, "")
	return s
}

// Basename returns the String instance with the basename of the current file path string,
// and trims the suffix based on the parameter(optional).
func (s *Str) Basename(suffix ...string) *Str {
	s.value = filepath.Base(s.value)
	if len(suffix) > 0 && suffix[0] != "" {
		s.value = strings.TrimSuffix(s.value, suffix[0])
	}
	return s
}

// Before returns the String instance with the substring before the first occurrence of the specified search string.
func (s *Str) Before(search string) *Str {
	index := strings.Index(s.value, search)
	if index != -1 {
		s.value = s.value[:index]
	}

	return s
}

// BeforeLast returns the String instance with the substring before the last occurrence of the specified search string.
func (s *Str) BeforeLast(search string) *Str {
	index := strings.LastIndex(s.value, search)
	if index != -1 {
		s.value = s.value[:index]
	}

	return s
}

// Between returns the String instance with the substring between the given start and end strings.
func (s *Str) Between(start, end string) *Str {
	if start == "" || end == "" {
		return s
	}
	return s.After(start).BeforeLast(end)
}

// BetweenFirst returns the String instance with the substring between the first occurrence of the given start string and the given end string.
func (s *Str) BetweenFirst(start, end string) *Str {
	if start == "" || end == "" {
		return s
	}
	return s.Before(end).After(start)
}

// Camel returns the String instance in camel case.
func (s *Str) Camel() *Str {
	return s.Studly().LcFirst()
}

// CharAt returns the character at the specified index.
func (s *Str) CharAt(index int) string {
	length := len(s.value)
	// return zero string when char doesn't exists
	if index < 0 && index < -length || index > length-1 {
		return ""
	}
	return Substr(s.value, index, 1)
}

// ChopEnd remove the given string(s) if it exists at the end of the haystack.
func (s *Str) ChopEnd(needle string, more ...string) *Str {
	more = append([]string{needle}, more...)

	for _, v := range more {
		if after, found := strings.CutSuffix(s.value, v); found {
			s.value = after
			break
		}
	}
	return s
}

// ChopStart remove the given string(s) if it exists at the start of the haystack.
func (s *Str) ChopStart(needle string, more ...string) *Str {
	more = append([]string{needle}, more...)

	for _, v := range more {
		if after, found := strings.CutPrefix(s.value, v); found {
			s.value = after
			break
		}
	}
	return s
}

// Contains returns true if the string contains the given value or any of the values.
func (s *Str) Contains(values ...string) bool {
	for _, value := range values {
		if value != "" && strings.Contains(s.value, value) {
			return true
		}
	}

	return false
}

// ContainsAll returns true if the string contains all of the given values.
func (s *Str) ContainsAll(values ...string) bool {
	for _, value := range values {
		if !strings.Contains(s.value, value) {
			return false
		}
	}

	return true
}

// Dirname returns the String instance with the directory name of the current file path string.
func (s *Str) Dirname(levels ...int) *Str {
	defaultLevels := 1
	if len(levels) > 0 {
		defaultLevels = levels[0]
	}

	dir := s.value
	for i := 0; i < defaultLevels; i++ {
		dir = filepath.Dir(dir)
	}

	s.value = dir
	return s
}

// EndsWith returns true if the string ends with the given value or any of the values.
func (s *Str) EndsWith(values ...string) bool {
	for _, value := range values {
		if value != "" && strings.HasSuffix(s.value, value) {
			return true
		}
	}

	return false
}

// Exactly returns true if the string is exactly the given value.
func (s *Str) Exactly(value string) bool {
	return s.value == value
}

// Excerpt returns the String instance truncated to the given length.
func (s *Str) Excerpt(phrase string, options ...excerptOption) *Str {
	defaultOptions := excerptOption{
		Radius:   100,
		Omission: "...",
	}

	if len(options) > 0 {
		if options[0].Radius != 0 {
			defaultOptions.Radius = options[0].Radius
		}
		if options[0].Omission != "" {
			defaultOptions.Omission = options[0].Omission
		}
	}

	radius := max(0, defaultOptions.Radius)
	omission := defaultOptions.Omission

	regex := regexp.MustCompile(`(.*?)(` + regexp.QuoteMeta(phrase) + `)(.*)`)
	matches := regex.FindStringSubmatch(s.value)

	if len(matches) == 0 {
		return s
	}

	start := strings.TrimRight(matches[1], "")
	end := strings.TrimLeft(matches[3], "")

	end = StringUtil.Of(Substr(end, 0, radius)).LTrim("").
		Unless(func(s *Str) bool {
			return s.Exactly(end)
		}, func(s *Str) *Str {
			return s.Append(omission)
		}).String()

	s.value = StringUtil.Of(Substr(start, max(len(start)-radius, 0), radius)).LTrim("").
		Unless(func(s *Str) bool {
			return s.Exactly(start)
		}, func(s *Str) *Str {
			return s.Prepend(omission)
		}).Append(matches[2], end).String()

	return s
}

// Explode splits the string by given delimiter string.
func (s *Str) Explode(delimiter string, limit ...int) []string {
	defaultLimit := 1
	isLimitSet := false
	if len(limit) > 0 && limit[0] != 0 {
		defaultLimit = limit[0]
		isLimitSet = true
	}
	tempExplode := strings.Split(s.value, delimiter)
	if !isLimitSet || len(tempExplode) <= defaultLimit {
		return tempExplode
	}

	if defaultLimit > 0 {
		return append(tempExplode[:defaultLimit-1], strings.Join(tempExplode[defaultLimit-1:], delimiter))
	}

	if defaultLimit < 0 && len(tempExplode) <= -defaultLimit {
		return []string{}
	}

	return tempExplode[:len(tempExplode)+defaultLimit]
}

// Finish returns the String instance with the given value appended.
// If the given value already ends with the suffix, it will not be added twice.
func (s *Str) Finish(value string) *Str {
	quoted := regexp.QuoteMeta(value)
	reg := regexp.MustCompile("(?:" + quoted + ")+$")
	s.value = reg.ReplaceAllString(s.value, "") + value
	return s
}

// Headline returns the String instance in headline case.
func (s *Str) Headline() *Str {
	parts := s.Explode(" ")

	if len(parts) > 1 {
		return s.Title()
	}

	parts = StringUtil.Of(strings.Join(parts, "_")).Studly().UcSplit()
	collapsed := StringUtil.Of(strings.Join(parts, "_")).
		Replace("-", "_").
		Replace(" ", "_").
		Replace("_", "_").Explode("_")

	s.value = strings.Join(collapsed, " ")
	return s
}

// Is returns true if the string matches any of the given patterns.
func (s *Str) Is(patterns ...string) bool {
	for _, pattern := range patterns {
		if pattern == s.value {
			return true
		}

		// Escape special characters in the pattern
		pattern = regexp.QuoteMeta(pattern)

		// Replace asterisks with regular expression wildcards
		pattern = strings.ReplaceAll(pattern, `\*`, ".*")

		// Create a regular expression pattern for matching
		regexPattern := "^" + pattern + "$"

		// Compile the regular expression
		regex := regexp.MustCompile(regexPattern)

		// Check if the value matches the pattern
		if regex.MatchString(s.value) {
			return true
		}
	}

	return false
}

// IsEmpty returns true if the string is empty.
func (s *Str) IsEmpty() bool {
	return s.value == ""
}

// IsNotEmpty returns true if the string is not empty.
func (s *Str) IsNotEmpty() bool {
	return !s.IsEmpty()
}

// IsAscii returns true if the string contains only ASCII characters.
func (s *Str) IsAscii() bool {
	return s.IsMatch(`^[\x00-\x7F]+$`)
}

// IsMap returns true if the string is a valid Map.
func (s *Str) IsMap() bool {
	var obj map[string]any
	return json.Unmarshal([]byte(s.value), &obj) == nil
}

// IsSlice returns true if the string is a valid Slice.
func (s *Str) IsSlice() bool {
	var arr []any
	return json.Unmarshal([]byte(s.value), &arr) == nil
}

// IsUlid returns true if the string is a valid ULID.
func (s *Str) IsUlid() bool {
	return s.IsMatch(`^[0-9A-Z]{26}$`)
}

// IsUuid returns true if the string is a valid UUID.
func (s *Str) IsUuid() bool {
	return s.IsMatch(`(?i)^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
}

// Kebab returns the String instance in kebab case.
func (s *Str) Kebab() *Str {
	return s.Snake("-")
}

// LcFirst returns the String instance with the first character lowercased.
func (s *Str) LcFirst() *Str {
	if s.Length() == 0 {
		return s
	}
	s.value = strings.ToLower(Substr(s.value, 0, 1)) + Substr(s.value, 1)
	return s
}

// Length returns the length of the string.
func (s *Str) Length() int {
	return utf8.RuneCountInString(s.value)
}

// Limit returns the String instance truncated to the given length.
func (s *Str) Limit(limit int, end ...string) *Str {
	defaultEnd := "..."
	if len(end) > 0 {
		defaultEnd = end[0]
	}

	if s.Length() <= limit {
		return s
	}
	s.value = Substr(s.value, 0, limit) + defaultEnd
	return s
}

// Lower returns the String instance in lower case.
func (s *Str) Lower() *Str {
	s.value = strings.ToLower(s.value)
	return s
}

// LTrim returns the String instance with the leftmost occurrence of the given value removed.
func (s *Str) LTrim(characters ...string) *Str {
	if len(characters) == 0 {
		s.value = strings.TrimLeft(s.value, " ")
		return s
	}

	s.value = strings.TrimLeft(s.value, characters[0])
	return s
}

// Mask returns the String instance with the given character masking the specified number of characters.
func (s *Str) Mask(character string, index int, length ...int) *Str {
	// Check if the character is empty, if so, return the original string.
	if character == "" {
		return s
	}

	segment := Substr(s.value, index, length...)

	// Check if the segment is empty, if so, return the original string.
	if segment == "" {
		return s
	}

	strLen := utf8.RuneCountInString(s.value)
	startIndex := index

	// Check if the start index is out of bounds.
	if index < 0 {
		if index < -strLen {
			startIndex = 0
		} else {
			startIndex = strLen + index
		}
	}

	start := Substr(s.value, 0, startIndex)
	segmentLen := utf8.RuneCountInString(segment)
	end := Substr(s.value, startIndex+segmentLen)

	s.value = start + strings.Repeat(Substr(character, 0, 1), segmentLen) + end
	return s
}

// Match returns the String instance with the first occurrence of the given pattern.
func (s *Str) Match(pattern string) *Str {
	if pattern == "" {
		return s
	}
	reg := regexp.MustCompile(pattern)
	s.value = reg.FindString(s.value)
	return s
}

// MatchAll returns all matches for the given regular expression.
func (s *Str) MatchAll(pattern string) []string {
	if pattern == "" {
		return []string{s.value}
	}
	reg := regexp.MustCompile(pattern)
	return reg.FindAllString(s.value, -1)
}

// IsMatch returns true if the string matches any of the given patterns.
func (s *Str) IsMatch(patterns ...string) bool {
	for _, pattern := range patterns {
		reg := regexp.MustCompile(pattern)
		if reg.MatchString(s.value) {
			return true
		}
	}

	return false
}

// NewLine appends one or more new lines to the current string.
func (s *Str) NewLine(count ...int) *Str {
	if len(count) == 0 {
		s.value += "\n"
		return s
	}

	s.value += strings.Repeat("\n", count[0])
	return s
}

// PadBoth returns the String instance padded to the left and right sides of the given length.
func (s *Str) PadBoth(length int, pad ...string) *Str {
	defaultPad := " "
	if len(pad) > 0 {
		defaultPad = pad[0]
	}
	short := max(0, length-s.Length())
	left := short / 2
	right := short/2 + short%2

	s.value = Substr(strings.Repeat(defaultPad, left), 0, left) + s.value + Substr(strings.Repeat(defaultPad, right), 0, right)

	return s
}

// PadLeft returns the String instance padded to the left side of the given length.
func (s *Str) PadLeft(length int, pad ...string) *Str {
	defaultPad := " "
	if len(pad) > 0 {
		defaultPad = pad[0]
	}
	short := max(0, length-s.Length())

	s.value = Substr(strings.Repeat(defaultPad, short), 0, short) + s.value
	return s
}

// PadRight returns the String instance padded to the right side of the given length.
func (s *Str) PadRight(length int, pad ...string) *Str {
	defaultPad := " "
	if len(pad) > 0 {
		defaultPad = pad[0]
	}
	short := max(0, length-s.Length())

	s.value = s.value + Substr(strings.Repeat(defaultPad, short), 0, short)
	return s
}

// Pipe passes the string to the given callback and returns the result.
func (s *Str) Pipe(callback func(s string) string) *Str {
	s.value = callback(s.value)
	return s
}

// Prepend one or more strings to the current string.
func (s *Str) Prepend(values ...string) *Str {
	s.value = strings.Join(values, "") + s.value
	return s
}

// Remove returns the String instance with the first occurrence of the given value removed.
func (s *Str) Remove(values ...string) *Str {
	for _, value := range values {
		s.value = strings.ReplaceAll(s.value, value, "")
	}

	return s
}

// Repeat returns the String instance repeated the given number of times.
func (s *Str) Repeat(times int) *Str {
	s.value = strings.Repeat(s.value, times)
	return s
}

// Replace returns the String instance with all occurrences of the search string replaced by the given replacement string.
func (s *Str) Replace(search string, replace string, caseSensitive ...bool) *Str {
	caseSensitive = append(caseSensitive, true)
	if len(caseSensitive) > 0 && !caseSensitive[0] {
		s.value = regexp.MustCompile("(?i)"+regexp.QuoteMeta(search)).ReplaceAllString(s.value, replace)
		return s
	}
	s.value = strings.ReplaceAll(s.value, search, replace)
	return s
}

// ReplaceEnd returns the String instance with the last occurrence of the given value replaced.
func (s *Str) ReplaceEnd(search string, replace string) *Str {
	if search == "" {
		return s
	}

	if s.EndsWith(search) {
		return s.ReplaceLast(search, replace)
	}

	return s
}

// ReplaceFirst returns the String instance with the first occurrence of the given value replaced.
func (s *Str) ReplaceFirst(search string, replace string) *Str {
	if search == "" {
		return s
	}
	s.value = strings.Replace(s.value, search, replace, 1)
	return s
}

// ReplaceLast returns the String instance with the last occurrence of the given value replaced.
func (s *Str) ReplaceLast(search string, replace string) *Str {
	if search == "" {
		return s
	}
	index := strings.LastIndex(s.value, search)
	if index != -1 {
		s.value = s.value[:index] + replace + s.value[index+len(search):]
		return s
	}

	return s
}

// ReplaceMatches returns the String instance with all occurrences of the given pattern
// replaced by the given replacement string.
func (s *Str) ReplaceMatches(pattern string, replace string) *Str {
	s.value = regexp.MustCompile(pattern).ReplaceAllString(s.value, replace)
	return s
}

// ReplaceStart returns the String instance with the first occurrence of the given value replaced.
func (s *Str) ReplaceStart(search string, replace string) *Str {
	if search == "" {
		return s
	}

	if s.StartsWith(search) {
		return s.ReplaceFirst(search, replace)
	}

	return s
}

// RTrim returns the String instance with the right occurrences of the given value removed.
func (s *Str) RTrim(characters ...string) *Str {
	if len(characters) == 0 {
		s.value = strings.TrimRight(s.value, " ")
		return s
	}

	s.value = strings.TrimRight(s.value, characters[0])
	return s
}

// Snake returns the String instance in snake case.
func (s *Str) Snake(delimiter ...string) *Str {
	defaultDelimiter := "_"
	if len(delimiter) > 0 {
		defaultDelimiter = delimiter[0]
	}
	words := fieldsFunc(s.value, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '-' || r == '_'
	}, func(r rune) bool {
		return unicode.IsUpper(r)
	})

	casesLower := cases.Lower(language.Und)
	var studlyWords []string
	for _, word := range words {
		studlyWords = append(studlyWords, casesLower.String(word))
	}

	s.value = strings.Join(studlyWords, defaultDelimiter)
	return s
}

// Split splits the string by given pattern string.
func (s *Str) Split(pattern string, limit ...int) []string {
	r := regexp.MustCompile(pattern)
	defaultLimit := -1
	if len(limit) != 0 {
		defaultLimit = limit[0]
	}

	return r.Split(s.value, defaultLimit)
}

// Squish returns the String instance with consecutive whitespace characters collapsed into a single space.
func (s *Str) Squish() *Str {
	leadWhitespace := regexp.MustCompile(`^[\s\p{Zs}]+|[\s\p{Zs}]+$`)
	insideWhitespace := regexp.MustCompile(`[\s\p{Zs}]{2,}`)
	s.value = leadWhitespace.ReplaceAllString(s.value, "")
	s.value = insideWhitespace.ReplaceAllString(s.value, " ")
	return s
}

// Start returns the String instance with the given value prepended.
func (s *Str) Start(prefix string) *Str {
	quoted := regexp.QuoteMeta(prefix)
	re := regexp.MustCompile(`^(` + quoted + `)+`)
	s.value = prefix + re.ReplaceAllString(s.value, "")
	return s
}

// StartsWith returns true if the string starts with the given value or any of the values.
func (s *Str) StartsWith(values ...string) bool {
	for _, value := range values {
		if strings.HasPrefix(s.value, value) {
			return true
		}
	}

	return false
}

// String returns the string value.
func (s *Str) String() string {
	return s.value
}

// Studly returns the String instance in studly case.
func (s *Str) Studly() *Str {
	words := fieldsFunc(s.value, func(r rune) bool {
		return r == '_' || r == ' ' || r == '-' || r == ',' || r == '.'
	}, func(r rune) bool {
		return unicode.IsUpper(r)
	})

	casesTitle := cases.Title(language.Und)
	var studlyWords []string
	for _, word := range words {
		studlyWords = append(studlyWords, casesTitle.String(word))
	}

	s.value = strings.Join(studlyWords, "")
	return s
}

// Substr returns the String instance starting at the given index with the specified length.
func (s *Str) Substr(start int, length ...int) *Str {
	s.value = Substr(s.value, start, length...)
	return s
}

// Swap replaces all occurrences of the search string with the given replacement string.
func (s *Str) Swap(replacements map[string]string) *Str {
	if len(replacements) == 0 {
		return s
	}

	oldNewPairs := make([]string, 0, len(replacements)*2)
	for k, v := range replacements {
		if k == "" {
			return s
		}
		oldNewPairs = append(oldNewPairs, k, v)
	}

	s.value = strings.NewReplacer(oldNewPairs...).Replace(s.value)
	return s
}

// Tap passes the string to the given callback and returns the string.
func (s *Str) Tap(callback func(Str)) *Str {
	callback(*s)
	return s
}

// Test returns true if the string matches the given pattern.
func (s *Str) Test(pattern string) bool {
	return s.IsMatch(pattern)
}

// Title returns the String instance in title case.
func (s *Str) Title() *Str {
	casesTitle := cases.Title(language.Und)
	s.value = casesTitle.String(s.value)
	return s
}

// Trim returns the String instance with trimmed characters from the left and right sides.
func (s *Str) Trim(characters ...string) *Str {
	if len(characters) == 0 {
		s.value = strings.TrimSpace(s.value)
		return s
	}

	s.value = strings.Trim(s.value, characters[0])
	return s
}

// UcFirst returns the String instance with the first character uppercased.
func (s *Str) UcFirst() *Str {
	if s.Length() == 0 {
		return s
	}
	s.value = strings.ToUpper(Substr(s.value, 0, 1)) + Substr(s.value, 1)
	return s
}

// UcSplit splits the string into words using uppercase characters as the delimiter.
func (s *Str) UcSplit() []string {
	words := fieldsFunc(s.value, func(r rune) bool {
		return false
	}, func(r rune) bool {
		return unicode.IsUpper(r)
	})
	return words
}

// Unless returns the String instance with the given fallback applied if the given condition is false.
func (s *Str) Unless(callback func(*Str) bool, fallback func(*Str) *Str) *Str {
	if !callback(s) {
		return fallback(s)
	}

	return s
}

// Upper returns the String instance in upper case.
func (s *Str) Upper() *Str {
	s.value = strings.ToUpper(s.value)
	return s
}

// When returns the String instance with the given callback applied if the given condition is true.
// If the condition is false, the fallback callback is applied (if provided).
func (s *Str) When(condition bool, callback ...func(*Str) *Str) *Str {
	if condition {
		if len(callback) > 0 && callback[0] != nil {
			return callback[0](s)
		}
	} else {
		if len(callback) > 1 && callback[1] != nil {
			return callback[1](s)
		}
	}

	return s
}

// WhenContains returns the String instance with the given callback applied if the string contains the given value.
func (s *Str) WhenContains(value string, callback ...func(*Str) *Str) *Str {
	return s.When(s.Contains(value), callback...)
}

// WhenContainsAll returns the String instance with the given callback applied if the string contains all the given values.
func (s *Str) WhenContainsAll(values []string, callback ...func(*Str) *Str) *Str {
	return s.When(s.ContainsAll(values...), callback...)
}

// WhenEmpty returns the String instance with the given callback applied if the string is empty.
func (s *Str) WhenEmpty(callback ...func(*Str) *Str) *Str {
	return s.When(s.IsEmpty(), callback...)
}

// WhenIsAscii returns the String instance with the given callback applied if the string contains only ASCII characters.
func (s *Str) WhenIsAscii(callback ...func(*Str) *Str) *Str {
	return s.When(s.IsAscii(), callback...)
}

// WhenNotEmpty returns the String instance with the given callback applied if the string is not empty.
func (s *Str) WhenNotEmpty(callback ...func(*Str) *Str) *Str {
	return s.When(s.IsNotEmpty(), callback...)
}

// WhenStartsWith returns the String instance with the given callback applied if the string starts with the given value.
func (s *Str) WhenStartsWith(value []string, callback ...func(*Str) *Str) *Str {
	return s.When(s.StartsWith(value...), callback...)
}

// WhenEndsWith returns the String instance with the given callback applied if the string ends with the given value.
func (s *Str) WhenEndsWith(value []string, callback ...func(*Str) *Str) *Str {
	return s.When(s.EndsWith(value...), callback...)
}

// WhenExactly returns the String instance with the given callback applied if the string is exactly the given value.
func (s *Str) WhenExactly(value string, callback ...func(*Str) *Str) *Str {
	return s.When(s.Exactly(value), callback...)
}

// WhenNotExactly returns the String instance with the given callback applied if the string is not exactly the given value.
func (s *Str) WhenNotExactly(value string, callback ...func(*Str) *Str) *Str {
	return s.When(!s.Exactly(value), callback...)
}

// WhenIs returns the String instance with the given callback applied if the string matches any of the given patterns.
func (s *Str) WhenIs(value string, callback ...func(*Str) *Str) *Str {
	return s.When(s.Is(value), callback...)
}

// WhenIsUlid returns the String instance with the given callback applied if the string is a valid ULID.
func (s *Str) WhenIsUlid(callback ...func(*Str) *Str) *Str {
	return s.When(s.IsUlid(), callback...)
}

// WhenIsUuid returns the String instance with the given callback applied if the string is a valid UUID.
func (s *Str) WhenIsUuid(callback ...func(*Str) *Str) *Str {
	return s.When(s.IsUuid(), callback...)
}

// WhenTest returns the String instance with the given callback applied if the string matches the given pattern.
func (s *Str) WhenTest(pattern string, callback ...func(*Str) *Str) *Str {
	return s.When(s.Test(pattern), callback...)
}

// WordCount returns the number of words in the string.
func (s *Str) WordCount() int {
	return len(strings.Fields(s.value))
}

// Words return the String instance truncated to the given number of words.
func (s *Str) Words(limit int, end ...string) *Str {
	defaultEnd := "..."
	if len(end) > 0 {
		defaultEnd = end[0]
	}

	words := strings.Fields(s.value)
	if len(words) <= limit {
		return s
	}

	s.value = strings.Join(words[:limit], " ") + defaultEnd
	return s
}

// Substr returns a substring of a given string, starting at the specified index
// and with a specified length.
// It handles UTF-8 encoded strings.
func Substr(str string, start int, length ...int) string {
	// Convert the string to a rune slice for proper handling of UTF-8 encoding.
	runes := []rune(str)
	strLen := utf8.RuneCountInString(str)
	end := strLen
	// Check if the start index is out of bounds.
	if start >= strLen {
		return ""
	}

	// If the start index is negative, count backwards from the end of the string.
	if start < 0 {
		start = max(strLen+start, 0)
	}

	if len(length) > 0 {
		if length[0] >= 0 {
			end = start + length[0]
		} else {
			end = strLen + length[0]
		}
	}

	// If the length is 0, return the substring from start to the end of the string.
	if len(length) == 0 {
		return string(runes[start:])
	}

	// Handle the case where lenArg is negative and less than start
	if end < start {
		return ""
	}

	if end > strLen {
		end = strLen
	}

	// Return the substring.
	return string(runes[start:end])
}

func Random(length int) string {
	s, _ := RandomSafe(length)
	return s
}

// RandomSafe 生成随机字符串，失败时返回错误。
func RandomSafe(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("RandomSafe: %w", err)
	}
	letters := "1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i, v := range b {
		b[i] = letters[v%byte(len(letters))]
	}

	return string(b), nil
}

// fieldsFunc splits the input string into words with preservation, following the rules defined by
// the provided functions f and preserveFunc.
func fieldsFunc(s string, f func(rune) bool, preserveFunc ...func(rune) bool) []string {
	var fields []string
	var currentField strings.Builder

	shouldPreserve := func(r rune) bool {
		for _, preserveFn := range preserveFunc {
			if preserveFn(r) {
				return true
			}
		}
		return false
	}

	runes := []rune(s)
	for i, r := range runes {
		if f(r) {
			if currentField.Len() > 0 {
				fields = append(fields, currentField.String())
				currentField.Reset()
			}
		} else if shouldPreserve(r) {
			// Smart uppercase handling for consecutive uppercase letters
			shouldSplit := false

			if i > 0 {
				prev := runes[i-1]
				var next rune
				if i < len(runes)-1 {
					next = runes[i+1]
				}

				// Split conditions:
				// 1. Previous char is not uppercase (covers lowercase, digits, symbols): "foo_B" -> "foo_" + "B"
				// 2. Current is uppercase, previous is uppercase, next is lowercase: "XMLHttp" -> "XML" + "Http"
				if !unicode.IsUpper(prev) {
					shouldSplit = true
				} else if unicode.IsUpper(prev) && unicode.IsLower(next) {
					shouldSplit = true
				}
			}

			if shouldSplit && currentField.Len() > 0 {
				fields = append(fields, currentField.String())
				currentField.Reset()
			}
			currentField.WriteRune(r)
		} else {
			currentField.WriteRune(r)
		}
	}

	if currentField.Len() > 0 {
		fields = append(fields, currentField.String())
	}

	return fields
}
