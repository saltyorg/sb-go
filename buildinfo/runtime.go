package buildinfo

// Linker-populated variables are intentionally private. Consumers receive an
// immutable value rather than sharing mutable process state.
var (
	version           string
	gitCommit         string
	uvVersion         = "0.12.3"
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
