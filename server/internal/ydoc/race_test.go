//go:build race

package ydoc_test

// raceDetector says the tests run under the race detector, which slows the
// code several times over, so a limit in seconds means nothing there.
const raceDetector = true
