package entry

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
	"unicode/utf8"
)

func FuzzAppendText(f *testing.F) {
	for _, seed := range []string{
		``, `plain`, `say \"hi\"`, `\\\/\b\f\n\r\t`, `caf\u00e9`, `\ud83d\ude00`, `\ud83d`, `\ude00\ud83d`,
		`\u001b[0m`, `a\qb`, `abc\`, `\u12`, "raw \xff byte", `\ud83d\u0041`, `\uD83D\uDE00\uDE00`,
		`\u0000`, `\uffff`, `\ud800\udc00`, `\udbff\udfff`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		raw = slices.Clip(raw) // a read past the end must panic, not find spare capacity
		got := Value{Raw: raw, Kind: KindString, Escaped: true}.AppendText(nil)

		// No escape decodes to more bytes than it takes.
		if len(got) > len(raw) {
			t.Fatalf("AppendText(%q) grew to %q", raw, got)
		}
		if bytes.IndexByte(raw, '\\') < 0 && !bytes.Equal(got, raw) {
			t.Fatalf("AppendText(%q) = %q, want the input: it has no escapes", raw, got)
		}

		quoted := append(append([]byte{'"'}, raw...), '"')
		if !json.Valid(quoted) {
			return
		}
		var want string
		if err := json.Unmarshal(quoted, &want); err != nil {
			t.Fatalf("json.Unmarshal(%q): %v", quoted, err)
		}
		if utf8.Valid(raw) && string(got) != want {
			t.Fatalf("AppendText(%q) = %q, encoding/json gives %q", raw, got, want)
		}
		// encoding/json also replaces every invalid byte with U+FFFD; AppendText leaves that to the renderer.
		if normalized := string([]rune(string(got))); normalized != want {
			t.Fatalf("AppendText(%q) = %q after replacing invalid bytes, encoding/json gives %q", raw, normalized, want)
		}
	})
}
