package runtime

var (
	// Version Build Var
	Version   string
	GitCommit string
	// DisableSelfUpdate is a build flag that disables the self-update functionality
	// Set this to "true" at build time to disable self-updates
	DisableSelfUpdate string
)
