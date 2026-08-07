package runtime

var (
	// Version Build Var
	Version   string
	GitCommit string
	// UVVersion is the exact uv release bundled with this sb-go build.
	UVVersion = "0.12.3"
	// DisableSelfUpdate is a build flag that disables the self-update functionality
	// Set this to "true" at build time to disable self-updates
	DisableSelfUpdate string
)
