package cli

import (
	"errors"
	"runtime/debug"
)

// ErrVersion is what Parse returns when the arguments ask for the version; like ErrHelp, it is a request.
var ErrVersion = errors.New("version requested")

// Version returns the line that --version prints.
func Version() string {
	return versionOf(debug.ReadBuildInfo())
}

// versionOf reads the version the go command stamped into the binary: the tag for "go install module@tag",
// a pseudo-version for a build from a checkout, nothing for "go run" and tests.
func versionOf(info *debug.BuildInfo, ok bool) string {
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "logpretty devel"
	}
	return "logpretty " + info.Main.Version
}
