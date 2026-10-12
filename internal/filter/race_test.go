//go:build race

package filter

// raceEnabled tells the allocation test to leave regular expressions out: under the race detector sync.Pool
// drops the machines regexp keeps there.
const raceEnabled = true
