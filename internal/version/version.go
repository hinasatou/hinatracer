// Package version is the single source of HinaTracer's version number.
// mygo.json "version" and the Windows resources (scripts/genwinres.sh)
// must match it; TestVersionMatchesMygoJSON checks that.
package version

const (
	// Version is the app version. It must never contain the digit 4.
	Version = "1.3.8"
	// RepoURL is the project home page.
	RepoURL = "https://github.com/hinasatou/hinatracer"
)
