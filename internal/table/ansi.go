package table

import (
	"strings"
	"unicode/utf8"

	runewidth "github.com/mattn/go-runewidth"
)

type ansiBlob []ansiSegment

func (a ansiBlob) Len() int {
	var c int
	for _, segment := range a {
		c += runewidth.StringWidth(segment.value)
	}
	return c
}

func (a ansiBlob) String() string {
	var output strings.Builder
	for _, segment := range a {
		output.WriteString(segment.style)
		output.WriteString(segment.value)
	}
	return output.String()
}

type ansiSegment struct {
	value string
	style string
}

func newANSI(input string) ansiBlob {
	var output []ansiSegment
	var current ansiSegment
	inCSI := false
	prev := rune(0)
	for _, r := range input {
		if inCSI {
			current.style += string(r)
			if r >= 0x40 && r <= 0x7E {
				inCSI = false
			}
		} else if r == '[' && prev == 0x1b {
			current.value = current.value[:utf8.RuneCountInString(current.value)-1]
			if current.value != "" {
				output = append(output, current)
				current = ansiSegment{}
			}
			inCSI = true
			current.style += "\x1b["
		} else {
			current.value = current.value + string(r)
		}
		prev = r
	}
	if current.value != "" || current.style != "" {
		output = append(output, current)
	}
	return output
}
