package cli

import (
	"errors"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/ai-fn/logpretty/internal/linereader"
)

func TestVersionOf(t *testing.T) {
	module := func(version string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/ai-fn/logpretty", Version: version}}
	}

	tests := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{name: "a release installed with go install", info: module("v0.1.0"), ok: true, want: "logpretty v0.1.0"},
		{
			name: "a build from a checkout",
			info: module("v0.0.0-20261012193210-1fee0d0e9524+dirty"),
			ok:   true,
			want: "logpretty v0.0.0-20261012193210-1fee0d0e9524+dirty",
		},
		{name: "a build without a version", info: module("(devel)"), ok: true, want: "logpretty devel"},
		{name: "an empty version", info: module(""), ok: true, want: "logpretty devel"},
		{name: "no build information", info: nil, ok: false, want: "logpretty devel"},
		{name: "build information that is not to be trusted", info: module("v9.9.9"), ok: false, want: "logpretty devel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionOf(tt.info, tt.ok); got != tt.want {
				t.Errorf("versionOf = %q, want %q", got, tt.want)
			}
		})
	}
}

// The test binary has build information of its own, whatever its version is.
func TestVersion(t *testing.T) {
	got := Version()
	name, version, ok := strings.Cut(got, " ")
	if !ok || name != "logpretty" || version == "" || strings.ContainsAny(version, " \t\n") {
		t.Errorf("Version() = %q, want the name, a space and a version", got)
	}
}

func TestParseVersion(t *testing.T) {
	for _, args := range [][]string{
		{"--version"},
		{"-version"},
		{"--version=true"},
		{"--utc", "--version"},
		{"--version", "--where", "a=1"},
		// An argument that is not a flag is not looked at.
		{"--version", "file.log"},
	} {
		got, err := Parse(args)
		if !errors.Is(err, ErrVersion) || !reflect.DeepEqual(got, Config{}) {
			t.Errorf("Parse(%q) = %+v, %v; want the zero Config and ErrVersion", args, got, err)
		}
	}

	if got, err := Parse([]string{"--version=false"}); err != nil || !reflect.DeepEqual(got, Config{MaxLine: linereader.DefaultMaxLine}) {
		t.Errorf("Parse(--version=false) = %+v, %v; want the defaults", got, err)
	}
	for _, args := range [][]string{{"--version", "--help"}, {"--help", "--version"}} {
		if _, err := Parse(args); !errors.Is(err, ErrHelp) {
			t.Errorf("Parse(%q) error = %v, want ErrHelp", args, err)
		}
	}
	for _, args := range [][]string{
		{"--nope", "--version"},
		{"--version", "--nope"},
		{"--version=maybe"},
		{"--", "--version"},
		// The flags end at the first argument that is not one, so the request is not seen.
		{"file.log", "--version"},
		// The value of a flag is checked wherever the flag stands.
		{"--max-line=0", "--version"},
		{"--version", "--max-line=0"},
		{"--where", "a=>1", "--version"},
		{"--version", "--where", "a=>1"},
	} {
		if _, err := Parse(args); err == nil || errors.Is(err, ErrVersion) {
			t.Errorf("Parse(%q) error = %v, want a failure that is not ErrVersion", args, err)
		}
	}
}
