package completion

import "bytes"

// identifiesCompletion distinguishes retired sb-go aliases from other commands
// in shared directories. Installation for an active name does not inspect it.
func identifiesCompletion(data []byte, shell Shell, name string) bool {
	if bytes.Contains(data, []byte(completionComment+string(shell)+" "+name+"\n")) ||
		bytes.Contains(data, []byte("# sb-go managed completion v1 "+string(shell)+" "+name+" ")) {
		return true
	}
	if bytes.Contains(data, []byte("# Custom tag completion for 'install' command with comma-separated support")) &&
		bytes.Contains(data, []byte("_"+name+"_custom_install_tags()")) {
		return true
	}
	if shell == Bash {
		return bytes.HasPrefix(data, []byte("# bash completion for "+name+" ")) &&
			bytes.Contains(data, []byte("_"+name+"_install_completion()"))
	}
	return shell == Zsh && bytes.HasPrefix(data, []byte("#compdef "+name+"\n\n# "+name+" zsh completion script\n")) &&
		bytes.Contains(data, []byte("_"+name+"_install_tags()"))
}
