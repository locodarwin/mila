// Package version holds build metadata, set at link time with -ldflags.
package version

// Version is the release version, such as "0.1.0". Builds without a tag
// report "dev".
var Version = "dev"

// Commit is the short git commit the binary was built from.
var Commit = "unknown"
