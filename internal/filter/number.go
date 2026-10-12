package filter

import (
	"bytes"
	"cmp"
	"strconv"
)

// number is a decimal number as it is written. An integer keeps its digits and is compared by them, so
// that an id past 2^53 does not match its neighbours, as it would after a trip through float64.
type number struct {
	text   []byte // the whole number
	digits []byte // of an integer: its digits without the sign and leading zeros; nil with a fraction or an exponent
	neg    bool   // of an integer: it is below zero
}

// parseNumber reads the whole text as a number: digits with an optional minus sign, fraction and exponent.
// It takes no more than a log writes: no plus sign, no hexadecimal, no "Inf".
func parseNumber(text []byte) (number, bool) {
	i := 0
	neg := len(text) > 0 && text[0] == '-'
	if neg {
		i = 1
	}
	start := i
	if i = skipDigits(text, i); i == start {
		return number{}, false
	}
	if i == len(text) {
		digits := text[start:]
		for len(digits) > 1 && digits[0] == '0' {
			digits = digits[1:]
		}
		// Minus zero is zero.
		return number{text: text, digits: digits, neg: neg && digits[0] != '0'}, true
	}

	if text[i] == '.' {
		start = i + 1
		if i = skipDigits(text, start); i == start {
			return number{}, false
		}
	}
	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		i++
		if i < len(text) && (text[i] == '+' || text[i] == '-') {
			i++
		}
		start = i
		if i = skipDigits(text, start); i == start {
			return number{}, false
		}
	}
	if i != len(text) {
		return number{}, false
	}
	return number{text: text}, true
}

// compare returns -1, 0 or 1 as a is below, equal to or above b. Two integers are compared exactly, whatever
// their length; with a fraction or an exponent on either side both are read as float64, so 2^53+1 equals 2^53.0.
func (a number) compare(b number) int {
	switch {
	case a.digits == nil || b.digits == nil:
		return cmp.Compare(a.float(), b.float())
	case a.neg != b.neg:
		if a.neg {
			return -1
		}
		return 1
	case a.neg:
		return compareDigits(b.digits, a.digits)
	}
	return compareDigits(a.digits, b.digits)
}

// compareDigits compares two integers written without a sign and leading zeros: the longer one is larger.
func compareDigits(a, b []byte) int {
	if len(a) != len(b) {
		return cmp.Compare(len(a), len(b))
	}
	return bytes.Compare(a, b)
}

// float reads the number as float64. The text is known to be a number; one out of range is an infinity or zero.
func (a number) float() float64 {
	f, _ := strconv.ParseFloat(string(a.text), 64)
	return f
}

func skipDigits(b []byte, i int) int {
	for i < len(b) && '0' <= b[i] && b[i] <= '9' {
		i++
	}
	return i
}
