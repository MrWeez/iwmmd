// Package version exposes build metadata injected at link time.
package version

import "fmt"

var (
	// Version is the release version or "dev".
	//
	// It is set with -ldflags at build time.
	Version = "dev"
	// Commit is the git commit SHA or "none".
	//
	// It is set with -ldflags at build time.
	Commit = "none"
	// Date is the build timestamp or "unknown".
	//
	// It is set with -ldflags at build time.
	Date = "unknown"
)

// String returns a human-readable build identifier.
func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
}
