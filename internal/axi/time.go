package axi

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var timestampPattern = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)$`)

type instant struct {
	second   time.Time
	fraction string
}

func parseInstant(value string) (instant, error) {
	m := timestampPattern.FindStringSubmatch(value)
	if m == nil || len(value) > 4096 {
		return instant{}, Usage("Expected a bounded RFC 3339 timestamp")
	}
	if m[3] != "Z" {
		h, _ := strconv.Atoi(m[3][1:3])
		minute, _ := strconv.Atoi(m[3][4:])
		if h > 23 || minute > 59 {
			return instant{}, Usage("Invalid RFC 3339 calendar timestamp")
		}
	}
	t, err := time.Parse(time.RFC3339, m[1]+m[3])
	if err != nil {
		return instant{}, Usage("Invalid RFC 3339 calendar timestamp")
	}
	f := strings.TrimRight(m[2], "0")
	for len(f) < 3 {
		f += "0"
	}
	return instant{t.UTC(), f}, nil
}
func instantString(t time.Time, f string) (string, error) {
	if t.Year() < 0 || t.Year() > 9999 {
		return "", Usage("Timestamp is outside the RFC 3339 year range")
	}
	return t.Format("2006-01-02T15:04:05") + "." + f + "Z", nil
}
func CanonicalTimestamp(v string) (string, error) {
	i, err := parseInstant(v)
	if err != nil {
		return "", err
	}
	return instantString(i.second, i.fraction)
}
func CompareTimestamps(a, b string) (int, error) {
	x, err := parseInstant(a)
	if err != nil {
		return 0, err
	}
	y, err := parseInstant(b)
	if err != nil {
		return 0, err
	}
	if x.second.Before(y.second) {
		return -1, nil
	}
	if x.second.After(y.second) {
		return 1, nil
	}
	n := max(len(x.fraction), len(y.fraction))
	af := x.fraction + strings.Repeat("0", n-len(x.fraction))
	bf := y.fraction + strings.Repeat("0", n-len(y.fraction))
	return strings.Compare(af, bf), nil
}
func ShiftTimestamp(v string, seconds int) (string, error) {
	i, err := parseInstant(v)
	if err != nil {
		return "", err
	}
	return instantString(i.second.Add(time.Duration(seconds)*time.Second), i.fraction)
}
func ProviderDate(v, condition string) (string, error) {
	i, err := parseInstant(v)
	if err != nil {
		return "", err
	}
	ms, _ := strconv.Atoi(i.fraction[:3])
	if condition == "before" && strings.Trim(i.fraction[3:], "0") != "" {
		ms++
	}
	t := i.second.Add(time.Duration(ms) * time.Millisecond)
	if t.Year() < 0 || t.Year() > 9999 {
		return "", nil
	}
	s := t.Format("20060102T150405")
	if t.Nanosecond() != 0 {
		s += t.Format(".000")
	}
	return s + "+0000", nil
}
