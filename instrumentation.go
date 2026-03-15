package re3

import "sync/atomic"

// Debug counters for the assertion/prefilter plan. Off by default; never enable
// in benchmark runs used for final numbers. See plan: Instrumentation first.
var (
	debugCountersEnabled atomic.Bool

	// DebugCounterCandidateStarts is incremented each time the assertion-path outer loop tries a new start.
	DebugCounterCandidateStarts atomic.Uint64
	// DebugCounterBytesScanned is incremented by the number of bytes scanned in each assertion-path attempt.
	DebugCounterBytesScanned atomic.Uint64
	// DebugCounterDerivativeCalls is incremented each time getNextState computes a derivative with non-zero matchContext.
	DebugCounterDerivativeCalls atomic.Uint64
)

// EnableDebugCounters turns on debug counters. Call only in tests or debug; leave false for production and benchmarks.
func EnableDebugCounters(on bool) {
	debugCountersEnabled.Store(on)
}

// DebugCountersEnabled reports whether debug counters are enabled.
func DebugCountersEnabled() bool {
	return debugCountersEnabled.Load()
}

// ResetDebugCounters zeros all debug counters.
func ResetDebugCounters() {
	DebugCounterCandidateStarts.Store(0)
	DebugCounterBytesScanned.Store(0)
	DebugCounterDerivativeCalls.Store(0)
}

func maybeCountCandidateStart() {
	if debugCountersEnabled.Load() {
		DebugCounterCandidateStarts.Add(1)
	}
}

func maybeCountBytesScanned(n int) {
	if n <= 0 || !debugCountersEnabled.Load() {
		return
	}
	DebugCounterBytesScanned.Add(uint64(n))
}

func maybeCountDerivativeCall() {
	if debugCountersEnabled.Load() {
		DebugCounterDerivativeCalls.Add(1)
	}
}
