// Package words tidies free text and applies the admin's language filter.
//
// Every free-text value (group name, display names, notes, the names Google
// reports) goes through Clean before it is stored. The admin's list of
// banned words or phrases then blocks new text that contains one, and masks
// any that is already stored, wherever it is shown. Storage safety doesn't
// depend on any of this: every query binds its values as parameters, and
// html/template escapes everything on the way out.
package words

import (
	"strings"
	"unicode"
)

// MaxList caps the admin's banned-word list.
const MaxList = 2000

// Mask replaces a banned word or phrase wherever text is displayed.
const Mask = "***"

// Clean collapses whitespace (newlines and tabs included) to single spaces
// and drops invisible control and formatting characters, such as the
// right-to-left override that can make a name display backwards. The zero
// width joiner is kept, because family and other combined emoji need it.
func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\u200d' {
			return r
		}
		if unicode.Is(unicode.Cc, r) && !unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// token is one word of a text, with where it sits in the original.
type token struct {
	start, end int // byte offsets
	word       string
}

// tokens splits text into words: runs of letters and digits, lower-cased.
// Everything else separates words, so matching is by whole word.
func tokens(s string) []token {
	var out []token
	start := -1
	for i, r := range s {
		word := unicode.IsLetter(r) || unicode.IsDigit(r)
		switch {
		case word && start < 0:
			start = i
		case !word && start >= 0:
			out = append(out, token{start, i, strings.ToLower(s[start:i])})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, token{start, len(s), strings.ToLower(s[start:])})
	}
	return out
}

// Filter is a set of banned words and phrases.
type Filter struct{ phrases [][]string }

// New builds a filter from a comma-separated list. Each entry may be a word
// or a phrase; case and punctuation are ignored.
func New(list string) *Filter {
	f := &Filter{}
	for _, entry := range strings.Split(list, ",") {
		var p []string
		for _, t := range tokens(entry) {
			p = append(p, t.word)
		}
		if len(p) > 0 {
			f.phrases = append(f.phrases, p)
		}
	}
	return f
}

// spans finds the byte ranges of s that match a banned phrase.
func (f *Filter) spans(s string) [][2]int {
	if f == nil || len(f.phrases) == 0 {
		return nil
	}
	ts := tokens(s)
	var out [][2]int
	for i := range ts {
	phrases:
		for _, p := range f.phrases {
			if i+len(p) > len(ts) {
				continue
			}
			for j, w := range p {
				if ts[i+j].word != w {
					continue phrases
				}
			}
			out = append(out, [2]int{ts[i].start, ts[i+len(p)-1].end})
		}
	}
	return out
}

// Blocked reports whether s contains a banned word or phrase.
func (f *Filter) Blocked(s string) bool { return len(f.spans(s)) > 0 }

// Mask replaces each banned word or phrase in s with ***.
func (f *Filter) Mask(s string) string {
	spans := f.spans(s)
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		if sp[0] < pos { // overlaps one already masked
			if sp[1] > pos {
				pos = sp[1]
			}
			continue
		}
		b.WriteString(s[pos:sp[0]])
		b.WriteString(Mask)
		pos = sp[1]
	}
	b.WriteString(s[pos:])
	return b.String()
}
