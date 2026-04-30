package api

import "runtime"

// BuildUserAgent returns the canonical CLI User-Agent header value:
//
//	urlbox-cli/<version> (<goos>/<goarch>) go/<runtime.Version>
//
// Empty version falls back to "0.0.0-dev" so unbuilt local binaries still
// produce a parseable header.
func BuildUserAgent(version string) string {
	if version == "" {
		version = "0.0.0-dev"
	}
	return "urlbox-cli/" + version +
		" (" + runtime.GOOS + "/" + runtime.GOARCH + ") " +
		"go/" + runtime.Version()
}
