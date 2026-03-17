package version

// Overridden at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
)

// Full returns version with commit hash (e.g., "0.1.0 (abc1234)").
func Full() string {
	if Commit == "unknown" || Commit == "" {
		return Version
	}
	return Version + " (" + Commit + ")"
}
