//go:build race

package dirview

// raceOn reports whether the race detector is built in. It slows code
// five to 10 times, so timing tests skip under it.
const raceOn = true
