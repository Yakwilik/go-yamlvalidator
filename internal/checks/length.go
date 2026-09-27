package checks

import "unicode/utf8"

// RuneLength counts Unicode code points, the length unit of native and tag rules.
func RuneLength(value string) int { return utf8.RuneCountInString(value) }

// ValidBounds reports whether optional inclusive length bounds are consistent.
func ValidBounds(minimum, maximum *int) bool {
	return (minimum == nil || *minimum >= 0) && (maximum == nil || *maximum >= 0) && (minimum == nil || maximum == nil || *minimum <= *maximum)
}
