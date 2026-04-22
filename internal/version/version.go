package version

var (
	// Version is the version of the CLI, set during build via ldflags.
	Version = "dev"
	// Commit is the git commit hash, set during build via ldflags.
	Commit = "none"
	// Date is the build date, set during build via ldflags.
	Date = "unknown"
)
