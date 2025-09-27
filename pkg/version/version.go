package version

var (
	Version    = "dev"
	CommitHash = "unknown"
	BuildDate  = "unknown"
)

// Summary returns a human-friendly version string for CLI output.
func Summary() string {
	return Version
}
