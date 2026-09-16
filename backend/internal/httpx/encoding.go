// Package httpx holds small HTTP helpers shared by the api and web packages.
package httpx

import (
	"strconv"
	"strings"
)

// Accepts reports whether coding is acceptable under an Accept-Encoding
// header. A qvalue of 0 rejects a coding, "*" stands in for codings the
// header does not name, and an absent header accepts nothing.
func Accepts(header, coding string) bool {
	var named, namedOK, star, starOK bool
	for part := range strings.SplitSeq(header, ",") {
		name, q := splitCoding(part)
		switch {
		case strings.EqualFold(name, coding):
			named, namedOK = true, q > 0
		case name == "*":
			star, starOK = true, q > 0
		}
	}
	if named {
		return namedOK
	}
	if star {
		return starOK
	}
	return false
}

// splitCoding splits one Accept-Encoding element such as "gzip;q=0.5" into
// its coding name and qvalue. An element without a qvalue has q=1.
func splitCoding(s string) (string, float64) {
	name, params, _ := strings.Cut(s, ";")
	q := 1.0
	for param := range strings.SplitSeq(params, ";") {
		k, v, ok := strings.Cut(param, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "q") {
			continue
		}
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			q = parsed
		}
	}
	return strings.TrimSpace(name), q
}
