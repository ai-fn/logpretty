package filter

import (
	"strings"
	"testing"
)

func TestParseNumber(t *testing.T) {
	valid := []string{
		"0", "-0", "7", "42", "007", "-15", "18446744073709551616", strings.Repeat("9", 100),
		"1.5", "-0.25", "0.0", "10.000", "1e3", "1E3", "1e+3", "1E-3", "-2.5e10", "00.5", "1e400", "1e-400",
	}
	for _, s := range valid {
		if _, ok := parseNumber([]byte(s)); !ok {
			t.Errorf("parseNumber(%q) is not a number", s)
		}
	}

	invalid := []string{
		"", "-", "+1", "1.", ".5", "-.5", "1.e3", "e3", "1e", "1e+", "1e-", "1e1.5", "1.2.3", "--1", "1-",
		"0x10", "0b1", "1_000", "1,000", "NaN", "Inf", "-Inf", "infinity", " 1", "1 ", "1\t", "12a", "a12", "1e3x", "١٢",
		"true", "null",
	}
	for _, s := range invalid {
		if n, ok := parseNumber([]byte(s)); ok || n.text != nil || n.digits != nil || n.neg {
			t.Errorf("parseNumber(%q) = %+v, %v; want the zero number and false", s, n, ok)
		}
	}
}

func TestNumberCompare(t *testing.T) {
	thirty := strings.Repeat("9", 30)

	tests := []struct {
		name string
		a, b string
		want int
	}{
		{name: "equal integers", a: "42", b: "42", want: 0},
		{name: "less", a: "41", b: "42", want: -1},
		{name: "greater", a: "43", b: "42", want: 1},
		{name: "more digits is more", a: "100", b: "99", want: 1},
		{name: "fewer digits is less", a: "9", b: "10", want: -1},
		{name: "leading zeros do not count", a: "0042", b: "42", want: 0},
		{name: "leading zeros do not make a number longer", a: "0099", b: "100", want: -1},
		{name: "zeros alone", a: "000", b: "0", want: 0},
		{name: "minus zero is zero", a: "-0", b: "0", want: 0},
		{name: "minus zero is above a negative", a: "-0", b: "-1", want: 1},
		{name: "negative below positive", a: "-1", b: "1", want: -1},
		{name: "negative below zero", a: "-1", b: "0", want: -1},
		{name: "positive above negative", a: "5", b: "-500", want: 1},
		{name: "negatives go the other way", a: "-10", b: "-9", want: -1},
		{name: "negatives of the same length", a: "-41", b: "-42", want: 1},
		{name: "equal negatives", a: "-42", b: "-0042", want: 0},

		{name: "the first integer float64 cannot hold", a: "9007199254740993", b: "9007199254740992", want: 1},
		{name: "the largest int64 and its neighbour", a: "9223372036854775807", b: "9223372036854775806", want: 1},
		{name: "one past int64", a: "9223372036854775808", b: "9223372036854775807", want: 1},
		{name: "the smallest int64 and one below", a: "-9223372036854775809", b: "-9223372036854775808", want: -1},
		{name: "the largest uint64 and one past it", a: "18446744073709551615", b: "18446744073709551616", want: -1},
		{name: "thirty digits and one more", a: thirty, b: "1" + strings.Repeat("0", 30), want: -1},
		{name: "thirty digits that differ in the last", a: thirty, b: thirty[:29] + "8", want: 1},
		{name: "thirty digits alike", a: thirty, b: thirty, want: 0},

		{name: "integer and the same with a fraction", a: "42", b: "42.0", want: 0},
		{name: "fraction and the same integer", a: "42.00", b: "42", want: 0},
		{name: "exponent", a: "1e2", b: "100", want: 0},
		{name: "exponent with a fraction", a: "1.5e1", b: "15", want: 0},
		{name: "negative exponent", a: "5E-1", b: "0.5", want: 0},
		{name: "integer below a fraction", a: "1", b: "1.5", want: -1},
		{name: "integer above a fraction", a: "2", b: "1.5", want: 1},
		{name: "negative fractions", a: "-2", b: "-1.5", want: -1},
		{name: "fractions", a: "0.25", b: "0.3", want: -1},
		{name: "fractions alike", a: "0.250", b: "0.25", want: 0},
		{name: "minus zero with a fraction", a: "-0.0", b: "0", want: 0},
		{name: "an integer past 2^53 against a fraction is read as float64", a: "9007199254740993", b: "9007199254740992.0", want: 0},
		{name: "thirty-one digits against an exponent are read as float64", a: "1" + strings.Repeat("0", 29) + "1", b: "1e30", want: 0},
		{name: "two numbers past the range of float64 are alike", a: "1e400", b: "1e500", want: 0},
		{name: "past the range of float64", a: "1e400", b: "1e308", want: 1},
		{name: "below the range of float64", a: "-1e400", b: "0", want: -1},
		{name: "too small for float64 is zero", a: "1e-400", b: "0", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, okA := parseNumber([]byte(tt.a))
			b, okB := parseNumber([]byte(tt.b))
			if !okA || !okB {
				t.Fatalf("parseNumber(%q), parseNumber(%q) = %v, %v; want numbers", tt.a, tt.b, okA, okB)
			}
			if got := a.compare(b); got != tt.want {
				t.Errorf("%s compare %s = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			if got := b.compare(a); got != -tt.want {
				t.Errorf("%s compare %s = %d, want %d", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}

// A number of more than 32 bytes read as float64 does allocate: the text is copied for strconv.
func TestNumberCompareDoesNotAllocate(t *testing.T) {
	integers := [][]byte{[]byte("42"), []byte("-9223372036854775809"), []byte("0042"), []byte(strings.Repeat("9", 60))}
	mixed := [][]byte{[]byte("42"), []byte("1.5e3"), []byte("-0.25"), []byte("18446744073709551616")}

	for name, texts := range map[string][][]byte{"integers": integers, "with fractions": mixed} {
		if allocs := testing.AllocsPerRun(100, func() {
			for _, x := range texts {
				a, _ := parseNumber(x)
				for _, y := range texts {
					b, _ := parseNumber(y)
					a.compare(b)
				}
			}
		}); allocs != 0 {
			t.Errorf("%s: parsing and comparing allocates %v times per round, want 0", name, allocs)
		}
	}
}
