//go:build race

package parse

// raceEnabled tells the allocation tests to leave json.Valid out: under the race detector sync.Pool
// drops a share of what it is given back, so the scanner of json.Valid is allocated anew.
const raceEnabled = true
