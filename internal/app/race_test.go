//go:build race

package app

// raceEnabled tells the allocation test to leave JSON out: under the race detector json.Valid allocates.
const raceEnabled = true
