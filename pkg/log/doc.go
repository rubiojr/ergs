package log

// Package log provides a very small opinionated wrapper around Go's standard
// library logging facilities. Its goal is to offer a consistent way to emit
// logs per service / datasource while keeping migration friction low.
//
// Key Features
//
//   - Per service / datasource loggers via ForService(name)
//   - Automatic prefix in every line: `[name]`  (example: `[github] repository synced`)
//   - Convenience level helpers: Infof, Warnf, Errorf, Debugf
//   - Debug logging can be enabled globally (SetGlobalDebug) or per service
//     (EnableDebugFor / DisableDebugFor)
//   - Uses the standard library *log.Logger* under the hood (no external deps)
//   - Central output writer (SetOutput) that updates existing loggers
//
// Non‑Goals (for now)
//
//   - Full-featured leveled logging framework
//   - Structured / JSON logging
//   - Log sampling, rotation, or asynchronous buffering
//
// These can be added later if explicitly requested. Keeping the surface minimal
// simplifies the incremental refactor away from directly using the stdlib log
// package across the codebase.
//
// Basic Usage
//
//	import (
//		"github.com/your/module/ergs/pkg/log"
//	)
//
//	func main() {
//		// Enable global debug logs if desired.
//		log.SetGlobalDebug(true)
//
//		// Acquire a logger for a datasource/service.
//		git := log.ForService("github")
//
//		git.Infof("starting sync")
//		git.Warnf("rate limit near exhaustion")
//		git.Debugf("detailed payload: %v", "...") // printed because global debug enabled
//	}
//
// Selective Debug
//
//	// Only enable debug for the 'github' service.
//	log.EnableDebugFor("github")
//	log.ForService("github").Debugf("visible")
//	log.ForService("rss").Debugf("NOT visible")
//
// Output Routing
//
//	// Send logs to a file (ensure proper closing in real code).
//	f, _ := os.Create("ergs.log")
//	log.SetOutput(f)
//
// Thread Safety
//
// All exported functions are safe for concurrent use. Internally the package
// relies on sync.Map and atomic primitives for minimal locking.
//
// Prefix Format
//
// The chosen prefix format `[name]` provides a concise, grep‑friendly service marker
// without timestamps when running under systemd (journald supplies them).
//
// Migration Strategy
//
//  1. Replace imports of the standard log package in a file with this package.
//  2. Obtain a local logger via ForService using an appropriate stable name
//     (e.g. the datasource slug).
//  3. Replace calls to log.Printf(...) with logger.Infof(...) or another
//     appropriate level helper.
//  4. Avoid introducing new direct stdlib log calls in refactored files.
//
// Testing
//
// Tests can redirect output by calling SetOutput with a bytes.Buffer,
// enabling assertions on log contents.
//
// Future Extensions
//
// The package intentionally exposes only what is needed now. Potential (yet
// intentionally deferred) enhancements:
//   - Structured fields: logger.With(k, v).Infof(...)
//   - JSON output mode
//   - Context propagation helpers
//   - Config-driven initialization
//
// Add these only when a concrete requirement emerges.
//
// Use responsibly and keep it minimal.
//
