package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/cache"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// getBinaryName returns the name the binary was invoked as (e.g., "sb" or "sb2")
func getBinaryName() string {
	binaryPath := os.Args[0]
	return filepath.Base(binaryPath)
}

// getCompletionPaths returns the appropriate completion paths based on the binary name
func getCompletionPaths() (bashPath, zshPath string) {
	cmdName := getBinaryName()
	bashPath = fmt.Sprintf("/etc/bash_completion.d/%s", cmdName)
	zshPath = fmt.Sprintf("/usr/share/zsh/vendor-completions/_%s", cmdName)
	return
}

// completionCmd represents the completion command
var completionCmd = &cobra.Command{
	Use:    "completion",
	Hidden: true,
	Short:  "Install shell completion for sb",
	Long: `Install shell completion scripts for sb.

This command installs completion scripts system-wide on Ubuntu.
Supported shells: bash, zsh

After installation, restart your shell or source the completion file.`,
}

// bashCompletionCmd installs bash completion
var bashCompletionCmd = &cobra.Command{
	Use:   "bash",
	Short: "Install bash completion",
	Long: `Installs bash completion script for the current binary name.

After installation, restart your shell or source the completion file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bashPath, _ := getCompletionPaths()
		cmdName := getBinaryName()

		// Install completion for the binary name used
		if err := installCompletion("bash", bashPath, func(path string) error {
			return generateStaticBashCompletion(path, cmdName)
		}); err != nil {
			return err
		}

		return nil
	},
}

// zshCompletionCmd installs zsh completion
var zshCompletionCmd = &cobra.Command{
	Use:   "zsh",
	Short: "Install zsh completion",
	Long: `Installs zsh completion script for the current binary name.

After installation, restart your shell or run:
  autoload -U compinit && compinit`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, zshPath := getCompletionPaths()
		cmdName := getBinaryName()

		// Install completion for the binary name used
		if err := installCompletion("zsh", zshPath, func(path string) error {
			return generateStaticZshCompletion(path, cmdName)
		}); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
	completionCmd.AddCommand(bashCompletionCmd)
	completionCmd.AddCommand(zshCompletionCmd)
}

// generateStaticBashCompletion creates a hybrid bash completion script:
// - Uses Cobra's native completion for all commands and subcommands
// - Adds custom tag completion logic for the 'install' command
func generateStaticBashCompletion(path, cmdName string) error {
	// Load cache and get tags for install command
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	tags := getCompletionTags(cacheInstance)
	if len(tags) == 0 {
		normalStyle := lipgloss.NewStyle()
		return fmt.Errorf("%s", normalStyle.Render(fmt.Sprintf("no tags found in cache - run '%s list' first to populate the cache", cmdName)))
	}

	// Temporarily set the root command's Use field to match the binary name
	// so Cobra generates completion with the correct command name
	originalUse := rootCmd.Use
	rootCmd.Use = cmdName
	defer func() { rootCmd.Use = originalUse }()

	// Create a temporary file to get Cobra's native completion
	tmpFile, err := os.CreateTemp("", "cobra-completion-*.bash")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Generate Cobra's native completion (without descriptions for cleaner output)
	if err := rootCmd.GenBashCompletionV2(tmpFile, false); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to generate bash completion: %w", err)
	}
	tmpFile.Close()

	// Read the generated completion
	cobraCompletion, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to read cobra completion: %w", err)
	}

	// Create the hybrid completion file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create completion file: %w", err)
	}
	defer file.Close()

	// Write Cobra's native completion first
	if _, err := file.Write(cobraCompletion); err != nil {
		return fmt.Errorf("failed to write cobra completion: %w", err)
	}

	// Append our custom install tag completion wrapper
	customInstallCompletion := fmt.Sprintf(`
# Custom tag completion for 'install' command with comma-separated support
_%s_custom_install_tags() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Static list of available tags
    local tags=(
%s
    )

    # Check if current word has commas without spaces - if so, reformat it
    if [[ "$cur" == *,* ]] && [[ "$cur" != *, ]]; then
        local reformatted=""
        IFS=',' read -ra parts <<< "$cur"
        local last_part=""
        local i

        for ((i=0; i<${#parts[@]}; i++)); do
            local part="${parts[i]}"
            part="${part# }"
            part="${part%% }"

            if [[ $i -eq $((${#parts[@]}-1)) ]]; then
                last_part="$part"
            else
                if [[ -n "$reformatted" ]]; then
                    reformatted="${reformatted}, ${part}"
                else
                    reformatted="${part}"
                fi
            fi
        done

        if [[ -n "$reformatted" ]]; then
            local prefix="${reformatted}, "
            cur="$last_part"

            local already_specified=()
            IFS=',' read -ra specified_tags <<< "$reformatted"
            for tag in "${specified_tags[@]}"; do
                tag="${tag# }"
                tag="${tag%% }"
                if [[ -n "$tag" ]]; then
                    already_specified+=("$tag")
                fi
            done

            local available_tags=()
            for tag in "${tags[@]}"; do
                local found=0
                for specified in "${already_specified[@]}"; do
                    if [[ "$tag" == "$specified" ]]; then
                        found=1
                        break
                    fi
                done
                if [[ $found -eq 0 ]]; then
                    available_tags+=("$tag")
                fi
            done

            local matches=()
            for tag in "${available_tags[@]}"; do
                if [[ "$tag" == "$cur"* ]]; then
                    matches+=("${prefix}${tag}")
                fi
            done

            COMPREPLY=("${matches[@]}")
            compopt -o nospace 2>/dev/null
            return
        fi
    fi

    # Get all already specified tags
    local already_specified=()
    local i
    for ((i=2; i<=$COMP_CWORD; i++)); do
        local word="${COMP_WORDS[i]}"
        word="${word%%,}"
        word="${word# }"
        word="${word%% }"

        if [[ -n "$word" ]] && [[ "$word" != "install" ]] && [[ $i -ne $COMP_CWORD ]]; then
            already_specified+=("$word")
        fi
    done

    if [[ "$cur" == *,* ]]; then
        local cur_prefix="${cur%%,*}"
        IFS=',' read -ra cur_tags <<< "$cur_prefix"
        for tag in "${cur_tags[@]}"; do
            tag="${tag# }"
            tag="${tag%% }"
            if [[ -n "$tag" ]]; then
                already_specified+=("$tag")
            fi
        done
    fi

    # Filter out already specified tags
    local available_tags=()
    for tag in "${tags[@]}"; do
        local found=0
        for specified in "${already_specified[@]}"; do
            if [[ "$tag" == "$specified" ]]; then
                found=1
                break
            fi
        done
        if [[ $found -eq 0 ]]; then
            available_tags+=("$tag")
        fi
    done

    # Handle comma-separated tags
    if [[ "$cur" == *,* ]]; then
        local prefix="${cur%%,*},"
        local partial="${cur##*,}"

        if [[ "$partial" == " "* ]]; then
            partial="${partial# }"
            prefix="${prefix} "
        fi

        local matches=()
        for tag in "${available_tags[@]}"; do
            if [[ "$tag" == "$partial"* ]]; then
                matches+=("${prefix}${tag}, ")
            fi
        done

        COMPREPLY=("${matches[@]}")
        compopt -o nospace 2>/dev/null
    elif [[ "$prev" == "," ]]; then
        COMPREPLY=($(compgen -W "${available_tags[*]}" -- "$cur"))

        local i
        for i in "${!COMPREPLY[@]}"; do
            COMPREPLY[$i]="${COMPREPLY[$i]}, "
        done

        compopt -o nospace 2>/dev/null
    else
        COMPREPLY=($(compgen -W "${available_tags[*]}" -- "$cur"))

        if [[ ${#COMPREPLY[@]} -eq 1 ]]; then
            compopt -o nospace 2>/dev/null
        fi
    fi
}

# Wrapper to override completion for 'install' command
_%s_custom() {
    local cur="${COMP_WORDS[COMP_CWORD]}"

    # Check if 'install' command is in the command line
    local has_install=0
    local i
    for ((i=1; i<COMP_CWORD; i++)); do
        if [[ "${COMP_WORDS[i]}" == "install" ]]; then
            has_install=1
            break
        fi
    done

    # If install command and we're past it, use custom tag completion
    if [[ $has_install -eq 1 ]] && [[ $COMP_CWORD -gt 1 ]]; then
        _%s_custom_install_tags
        return
    fi

    # Otherwise use Cobra's native completion
    __start_%s
}

# Replace the default completion function with our custom wrapper
complete -o default -F _%s_custom %s
`, cmdName, formatTagsForBash(tags), cmdName, cmdName, cmdName, cmdName, cmdName)

	if _, err := file.WriteString(customInstallCompletion); err != nil {
		return fmt.Errorf("failed to write custom completion: %w", err)
	}

	return nil
}

// formatTagsForBash formats tags array for bash script
func formatTagsForBash(tags []string) string {
	var lines []string
	for _, tag := range tags {
		lines = append(lines, fmt.Sprintf("        %q", tag))
	}
	return strings.Join(lines, "\n")
}

// generateStaticZshCompletion creates a hybrid zsh completion script:
// - Uses Cobra's native completion for all commands and subcommands
// - Adds custom tag completion logic for the 'install' command
func generateStaticZshCompletion(path, cmdName string) error {
	// Load cache and get tags for install command
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	tags := getCompletionTags(cacheInstance)
	if len(tags) == 0 {
		normalStyle := lipgloss.NewStyle()
		return fmt.Errorf("%s", normalStyle.Render(fmt.Sprintf("no tags found in cache - run '%s list' first to populate the cache", cmdName)))
	}

	// Temporarily set the root command's Use field to match the binary name
	// so Cobra generates completion with the correct command name
	originalUse := rootCmd.Use
	rootCmd.Use = cmdName
	defer func() { rootCmd.Use = originalUse }()

	// Create a temporary file to get Cobra's native completion
	tmpFile, err := os.CreateTemp("", "cobra-completion-*.zsh")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Generate Cobra's native completion
	if err := rootCmd.GenZshCompletion(tmpFile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to generate zsh completion: %w", err)
	}
	tmpFile.Close()

	// Read the generated completion
	cobraCompletion, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to read cobra completion: %w", err)
	}

	// Create the hybrid completion file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create completion file: %w", err)
	}
	defer file.Close()

	// Write Cobra's native completion first
	if _, err := file.Write(cobraCompletion); err != nil {
		return fmt.Errorf("failed to write cobra completion: %w", err)
	}

	// Append our custom install tag completion wrapper
	customInstallCompletion := fmt.Sprintf(`
# Custom tag completion for 'install' command with comma-separated support
_%s_custom_install_tags() {
    local cur_word="${words[CURRENT]}"
    local -a already_specified available_tags

    # Static list of available tags
    local -a tags
    tags=(
%s
    )

    # Function to extract all already specified tags
    _extract_specified_tags() {
        local word part
        already_specified=()

        for ((i=3; i<CURRENT; i++)); do
            word="${words[i]}"
            if [[ "$word" == *,* ]]; then
                for part in ${(s:,:)word}; do
                    part="${part## }"
                    part="${part%%%% }"
                    [[ -n "$part" ]] && already_specified+=("$part")
                done
            else
                word="${word%%,}"
                word="${word## }"
                word="${word%%%% }"
                [[ -n "$word" ]] && already_specified+=("$word")
            fi
        done

        if [[ "$cur_word" == *,* ]]; then
            local prefix="${cur_word%%,*}"
            for part in ${(s:,:)prefix}; do
                part="${part## }"
                part="${part%%%% }"
                [[ -n "$part" ]] && already_specified+=("$part")
            done
        fi
    }

    _extract_specified_tags

    # Filter available tags
    available_tags=()
    for tag in $tags; do
        local found=0
        for specified in $already_specified; do
            if [[ "$tag" == "$specified" ]]; then
                found=1
                break
            fi
        done
        [[ $found -eq 0 ]] && available_tags+=("$tag")
    done

    # Handle comma-separated input
    if [[ "$cur_word" == *,* ]]; then
        local needs_reformat=0
        local test_word="$cur_word"

        if [[ "$test_word" =~ ',[^ ]' ]] && [[ "$test_word" != *, ]]; then
            needs_reformat=1
        fi

        if [[ $needs_reformat -eq 1 ]]; then
            local reformatted=""
            local parts=(${(s:,:)cur_word})
            local last_part=""

            for ((i=1; i<=$#parts; i++)); do
                local part="${parts[i]}"
                part="${part## }"
                part="${part%%%% }"

                if [[ $i -eq $#parts ]]; then
                    last_part="$part"
                else
                    if [[ -n "$reformatted" ]]; then
                        reformatted="${reformatted}, ${part}"
                    else
                        reformatted="${part}"
                    fi
                fi
            done

            if [[ -n "$reformatted" ]]; then
                local prefix="${reformatted}, "
                local -a matches

                for tag in $available_tags; do
                    if [[ "$tag" == ${last_part}* ]]; then
                        matches+=("${prefix}${tag}")
                    fi
                done

                if [[ ${#matches} -gt 0 ]]; then
                    compadd -U -Q -S ', ' -- $matches
                    return
                fi
            fi
        else
            local prefix="${cur_word%%,*},"
            local partial="${cur_word##*,}"

            if [[ "$partial" == " "* ]]; then
                partial="${partial# }"
                prefix="${prefix} "
            else
                prefix="${prefix} "
            fi

            local -a matches
            for tag in $available_tags; do
                if [[ "$tag" == ${partial}* ]]; then
                    matches+=("${prefix}${tag}")
                fi
            done

            if [[ ${#matches} -gt 0 ]]; then
                compadd -U -Q -S ', ' -- $matches
                return
            fi
        fi
    else
        if [[ ${#available_tags} -gt 0 ]]; then
            local -a matching_tags
            for tag in $available_tags; do
                if [[ "$tag" == ${cur_word}* ]]; then
                    matching_tags+=("$tag")
                fi
            done

            if [[ ${#matching_tags} -gt 0 ]]; then
                compadd -Q -S ', ' -- $matching_tags
            fi
        fi
    fi
}

# Wrapper function to override install command completion
_%s_custom() {
    local line state

    _arguments -C \
        "1: :->cmds" \
        "*::arg:->args"

    case "$state" in
        cmds)
            _%s
            ;;
        args)
            case ${line[1]} in
                install)
                    _%s_custom_install_tags
                    ;;
                *)
                    _%s
                    ;;
            esac
            ;;
    esac
}

# Replace default completion with custom wrapper
compdef _%s_custom %s
`, cmdName, formatTagsForZsh(tags), cmdName, cmdName, cmdName, cmdName, cmdName, cmdName)

	if _, err := file.WriteString(customInstallCompletion); err != nil {
		return fmt.Errorf("failed to write custom completion: %w", err)
	}

	return nil
}

// formatTagsForZsh formats tags array for zsh script
func formatTagsForZsh(tags []string) string {
	var lines []string
	for _, tag := range tags {
		lines = append(lines, fmt.Sprintf("    %q", tag))
	}
	return strings.Join(lines, "\n")
}

// installCompletion handles the common logic for installing completion scripts
func installCompletion(shellName, targetPath string, generateFunc func(string) error) error {
	// Note: Root check not needed - main.go automatically elevates to root

	// Ensure the target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	// Generate the completion file
	if err := generateFunc(targetPath); err != nil {
		return fmt.Errorf("failed to generate %s completion: %w", shellName, err)
	}

	fmt.Printf("✓ %s completion installed to %s\n", shellName, targetPath)

	// Provide shell-specific reload instructions
	switch shellName {
	case "bash":
		fmt.Println("\nTo enable completions in your current shell, run:")
		fmt.Printf("  source %s\n", targetPath)
		fmt.Println("\nOr restart your shell.")
	case "zsh":
		fmt.Println("\nTo enable completions in your current shell, run:")
		fmt.Println("  autoload -U compinit && compinit")
		fmt.Println("\nOr restart your shell.")
	}

	return nil
}

// isZshInstalled checks if zsh is installed by checking if the vendor-completions directory exists
func isZshInstalled() bool {
	_, err := os.Stat("/usr/share/zsh/vendor-completions/")
	return err == nil
}

// InstallOrRegenerateCompletion installs or regenerates a completion file
// This is used by the update command to auto-install or update completions
func InstallOrRegenerateCompletion(shellName, targetPath string, generateFunc func(string) error) error {
	// Check if we have write permissions
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		// Can't write, skip silently
		return nil
	}

	// Generate the completion file
	if err := generateFunc(targetPath); err != nil {
		// Generation failed, skip silently
		return nil
	}

	return nil
}

// RegenerateCompletion regenerates a completion file if it exists
// This is used by the update command to keep completions in sync
func RegenerateCompletion(shellName, targetPath string, generateFunc func(string) error) error {
	// Check if the completion file exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		// File doesn't exist, skip silently
		return nil
	}

	// Check if we have write permissions
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		// Can't write, skip silently
		return nil
	}

	// Generate the completion file
	if err := generateFunc(targetPath); err != nil {
		// Generation failed, skip silently
		return nil
	}

	// Success - could add verbose logging here later
	// fmt.Printf("Updated %s completion at %s\n", shellName, targetPath)

	return nil
}
