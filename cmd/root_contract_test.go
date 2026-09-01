package cmd

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCommandTreeContract(t *testing.T) {
	const want = "4377dbe6273983c9bcd6a31725be724cb79ab5b27314b822aacf94488974a7f0"
	contract := commandTreeContract(NewRootCommand(Dependencies{}))
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(contract)))
	if got != want {
		t.Fatalf("command tree contract hash = %s, want %s\n\n%s", got, want, contract)
	}
}

func commandTreeContract(root *cobra.Command) string {
	var lines []string
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		lines = append(lines, fmt.Sprintf(
			"command path=%q use=%q aliases=%q hidden=%t short=%q long=%q valid=%q argAliases=%q suggest=%q disableFlagsInUseLine=%t silenceUsage=%t silenceErrors=%t completionDisabled=%t annotations=%q",
			command.CommandPath(), command.Use, command.Aliases, command.Hidden, command.Short, command.Long,
			command.ValidArgs, command.ArgAliases, command.SuggestFor, command.DisableFlagsInUseLine,
			command.SilenceUsage, command.SilenceErrors, command.CompletionOptions.DisableDefaultCmd,
			sortedMap(command.Annotations),
		))
		for count := 0; count <= 4; count++ {
			args := make([]string, count)
			for i := range args {
				args[i] = fmt.Sprintf("arg%d", i+1)
			}
			var result string
			if command.Args == nil {
				result = "nil"
			} else if err := command.Args(command, args); err != nil {
				result = err.Error()
			} else {
				result = "ok"
			}
			lines = append(lines, fmt.Sprintf("args path=%q count=%d result=%q", command.CommandPath(), count, result))
		}
		appendFlags := func(scope string, flags *pflag.FlagSet) {
			flags.VisitAll(func(flag *pflag.Flag) {
				lines = append(lines, fmt.Sprintf(
					"flag path=%q scope=%s name=%q shorthand=%q type=%q default=%q noOpt=%q usage=%q hidden=%t deprecated=%q annotations=%q",
					command.CommandPath(), scope, flag.Name, flag.Shorthand, flag.Value.Type(), flag.DefValue,
					flag.NoOptDefVal, flag.Usage, flag.Hidden, flag.Deprecated, sortedMap(flag.Annotations),
				))
			})
		}
		appendFlags("local", command.LocalNonPersistentFlags())
		appendFlags("persistent", command.PersistentFlags())

		children := append([]*cobra.Command(nil), command.Commands()...)
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			walk(child)
		}
	}
	walk(root)
	return strings.Join(lines, "\n")
}

func sortedMap[V any](values map[string]V) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, values[key]))
	}
	return strings.Join(parts, ",")
}
