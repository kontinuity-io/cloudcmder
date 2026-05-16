package screens

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// matchHighlight is the ANSI wrapper applied to fuzzy-matched runes inside
// the NAME cell. Bold + underline reads loud enough to spot at a glance
// without being eye-piercing on long names.
var matchHighlight = lipgloss.NewStyle().Bold(true).Underline(true)

// highlightName wraps each matched rune in name with the highlight style.
// matchedByteIndexes are sorted byte offsets into the multi-field row
// corpus (name|region|status|labels); only offsets that fall inside name's
// byte range are applied — the rest belong to other corpus segments.
//
// ASCII fast path: the common case for cloud resource names (no multi-byte
// runes) goes through a single string-builder pass. Non-ASCII names walk
// utf8 once to map byte-offsets to rune-indexes, then build the output.
func highlightName(name string, matchedByteIndexes []int) string {
	if name == "" || len(matchedByteIndexes) == 0 {
		return name
	}
	nameByteLen := len(name)

	// Build a set of rune indexes inside name that are highlighted.
	matchedRunes := make(map[int]bool)
	for _, b := range matchedByteIndexes {
		if b < 0 || b >= nameByteLen {
			continue
		}
		matchedRunes[byteOffsetToRuneIndex(name, b)] = true
	}

	if len(matchedRunes) == 0 {
		return name
	}

	var sb strings.Builder
	for i, r := range name {
		// `i` here is the byte offset of the rune (Go's range-over-string
		// semantics). Convert to rune index by counting how many runes
		// preceded this byte offset.
		runeIdx := byteOffsetToRuneIndex(name, i)
		if matchedRunes[runeIdx] {
			sb.WriteString(matchHighlight.Render(string(r)))
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// byteOffsetToRuneIndex returns the rune index that starts at the given byte
// offset in s. Returns the rune count (== last index + 1) if the offset is
// past the string. Linear walk — O(n); fine for short cell strings.
func byteOffsetToRuneIndex(s string, byteOffset int) int {
	runes := 0
	off := 0
	for off < byteOffset && off < len(s) {
		_, sz := utf8.DecodeRuneInString(s[off:])
		off += sz
		runes++
	}
	return runes
}
