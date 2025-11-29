package runtime

import "fmt"

var (
	// Version is the semantic version (set via -ldflags)
	Version = "0.0.0-dev"

	// GitCommit is the short git commit hash (set via -ldflags)
	GitCommit = "dev"

	// BuildTime is the UTC build timestamp (set via -ldflags)
	BuildTime = "unknown"
)

// VersionString returns the formatted version string for display.
func VersionString() string {
	return fmt.Sprintf("sb-docs version %s (%s) built %s", Version, GitCommit, BuildTime)
}
