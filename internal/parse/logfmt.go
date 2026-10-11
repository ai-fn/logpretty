package parse

import "github.com/ai-fn/logpretty/internal/entry"

const (
	// minLogfmtPairs keeps a sentence with one key=value in it, like "listening on port=8080", plain.
	minLogfmtPairs = 2
	// logfmtCoverage is the share of token bytes, in percent, that pairs must make up from the first pair on.
	logfmtCoverage = 70
)

// logfmtToken is a whitespace-separated piece of a line, as offsets into it.
type logfmtToken struct {
	start, end       int // the whole token
	keyEnd           int // pairs only: the index of the equals sign
	valStart, valEnd int // pairs only: the value without its quotes
	pair             bool
	escaped          bool
}

// appendLogfmtFields appends the fields of a logfmt line to dst and returns the text before the first pair.
// A line that is not mostly pairs is rejected: dst comes back at its old length, its spare room used as scratch.
func appendLogfmtFields(dst []entry.Field, line []byte) (fields []entry.Field, prefix []byte, ok bool) {
	fields = dst
	var pairs, pairBytes, otherBytes, prefixStart int
	// Capacity is capped on every slice handed out, so that an append cannot overwrite the line.
	for tok, more := nextLogfmtToken(line, 0); more; tok, more = nextLogfmtToken(line, tok.end) {
		switch {
		case tok.pair:
			pairs++
			pairBytes += tok.end - tok.start
			fields = append(fields, entry.Field{
				Key:   line[tok.start:tok.keyEnd:tok.keyEnd],
				Value: entry.Value{Raw: line[tok.valStart:tok.valEnd:tok.valEnd], Kind: entry.KindString, Escaped: tok.escaped},
			})
		case pairs == 0:
			// A timestamp or a container name in front of the pairs says nothing about the format.
			if prefix == nil {
				prefixStart = tok.start
			}
			prefix = line[prefixStart:tok.end:tok.end]
		default:
			otherBytes += tok.end - tok.start
			fields = append(fields, entry.Field{Key: line[tok.start:tok.end:tok.end]})
		}
	}
	// int64, so that a line of tens of megabytes cannot overflow on a 32-bit platform.
	if pairs < minLogfmtPairs || int64(pairBytes)*100 < logfmtCoverage*int64(pairBytes+otherBytes) {
		// The grown slice goes back, so that the next line like this one does not grow it again.
		return fields[:len(dst)], nil, false
	}
	return fields, prefix, true
}

// nextLogfmtToken returns the first token at or after line[i]. It reports false when only whitespace is left.
func nextLogfmtToken(line []byte, i int) (tok logfmtToken, ok bool) {
	for i < len(line) && isSpace(line[i]) {
		i++
	}
	if i == len(line) {
		return tok, false
	}
	tok.start = i

	eq := i
	if isLogfmtKeyStart(line[i]) {
		eq++
		for eq < len(line) && isLogfmtKeyByte(line[eq]) {
			eq++
		}
	}
	if eq == i || eq == len(line) || line[eq] != '=' {
		tok.end = wordEnd(line, eq)
		return tok, true
	}

	val := eq + 1
	if val < len(line) && line[val] == '"' {
		end, escaped := quoteEnd(line, val+1)
		if end < 0 {
			// Without the closing quote nothing says where the value ends: the rest of the line is one token.
			tok.end = len(line)
			for isSpace(line[tok.end-1]) {
				tok.end--
			}
			return tok, true
		}
		tok.keyEnd, tok.valStart, tok.valEnd, tok.end = eq, val+1, end, end+1
		tok.pair, tok.escaped = true, escaped
		return tok, true
	}
	tok.keyEnd, tok.valStart, tok.valEnd = eq, val, wordEnd(line, val)
	tok.end = tok.valEnd
	tok.pair = true
	return tok, true
}

// quoteEnd returns the index of the quote that closes the value whose text starts at line[start], or -1,
// and whether the text has escapes.
func quoteEnd(line []byte, start int) (end int, escaped bool) {
	for end = start; end < len(line); end++ {
		switch line[end] {
		case '"':
			return end, escaped
		case '\\':
			escaped = true
			end++
		}
	}
	return -1, false
}

// wordEnd returns the index of the first whitespace at or after line[i], or the length of the line.
func wordEnd(line []byte, i int) int {
	for i < len(line) && !isSpace(line[i]) {
		i++
	}
	return i
}

// isLogfmtKeyStart reports whether c can start a key: a letter, '_', '@' or any byte from 0x80 up, which
// takes in invalid UTF-8 and Unicode spaces alike.
func isLogfmtKeyStart(c byte) bool {
	return c >= 0x80 || c == '_' || c == '@' || 'a' <= c|0x20 && c|0x20 <= 'z'
}

// isLogfmtKeyByte reports whether c can go on a key. The set is narrow, so that a URL like "http://x?a=1"
// is not a pair.
func isLogfmtKeyByte(c byte) bool {
	return isLogfmtKeyStart(c) || c == '.' || c == '-' || '0' <= c && c <= '9'
}
