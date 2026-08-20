package buildinfo

// Build metadata is intentionally private. Consumers receive an immutable
// value rather than sharing mutable process state. The uv version is pinned in
// source so raw builds, tests, and release builds all use the same value.
const uvVersion = "0.12.3"

var (
	version           string
	gitCommit         string
	disableSelfUpdate string
)

type Info struct {
	Version           string
	GitCommit         string
	UVVersion         string
	DisableSelfUpdate bool
}

func Current() Info {
	return Info{
		Version:           version,
		GitCommit:         gitCommit,
		UVVersion:         uvVersion,
		DisableSelfUpdate: disableSelfUpdate == "true",
	}
}
