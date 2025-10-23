package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/spf13/cobra"
)

// Completion file paths for Ubuntu system-wide installation
const (
	bashCompletionPath      = "/etc/bash_completion.d/sb"
	bashCompletionAliasPath = "/etc/bash_completion.d/sb2"
	zshCompletionPath       = "/usr/share/zsh/vendor-completions/_sb"
	zshCompletionAliasPath  = "/usr/share/zsh/vendor-completions/_sb2"
)

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
	Short: "Install bash completion for sb",
	Long: `Installs bash completion script to /etc/bash_completion.d/sb

After installation, restart your shell or run:
  source /etc/bash_completion.d/sb`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Install main completion
		if err := installCompletion("bash", bashCompletionPath, generateBashCompletion); err != nil {
			return err
		}

		// Install alias completion if alias is different from "sb"
		if constants.CompletionAlias != "sb" {
			if err := installCompletion("bash", bashCompletionAliasPath, generateBashCompletionAlias); err != nil {
				return err
			}
		}

		return nil
	},
}

// zshCompletionCmd installs zsh completion
var zshCompletionCmd = &cobra.Command{
	Use:   "zsh",
	Short: "Install zsh completion for sb",
	Long: `Installs zsh completion script to /usr/share/zsh/vendor-completions/_sb

After installation, restart your shell or run:
  autoload -U compinit && compinit`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Install main completion
		if err := installCompletion("zsh", zshCompletionPath, generateZshCompletion); err != nil {
			return err
		}

		// Install alias completion if alias is different from "sb"
		if constants.CompletionAlias != "sb" {
			if err := installCompletion("zsh", zshCompletionAliasPath, generateZshCompletionAlias); err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
	completionCmd.AddCommand(bashCompletionCmd)
	completionCmd.AddCommand(zshCompletionCmd)
}

// generateBashCompletion generates bash completion for the main 'sb' command
func generateBashCompletion(path string) error {
	return generateStaticBashCompletion(path, "sb")
}

// generateStaticBashCompletion creates a static completion script with tags embedded
func generateStaticBashCompletion(path, cmdName string) error {
	// Load cache and get tags
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	// Get tags from cache
	tags := getCompletionTags(cacheInstance)
	if len(tags) == 0 {
		return fmt.Errorf("no tags found in cache - run 'sb list' first to populate the cache")
	}

	// Generate the static completion script
	script := fmt.Sprintf(`# bash completion for %s                                   -*- shell-script -*-

_%s_install_completion() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Static list of available tags
    local tags=(
%s
    )

    # Check if current word has commas without spaces - if so, reformat it
    if [[ "$cur" == *,* ]] && [[ "$cur" != *, ]]; then
        # The current word has commas but doesn't end with comma
        # Split it and reformat with spaces
        local reformatted=""
        IFS=',' read -ra parts <<< "$cur"
        local last_part=""
        local i

        for ((i=0; i<${#parts[@]}; i++)); do
            local part="${parts[i]}"
            # Trim any existing spaces
            part="${part# }"
            part="${part%% }"

            if [[ $i -eq $((${#parts[@]}-1)) ]]; then
                # This is the last part (potentially incomplete)
                last_part="$part"
            else
                # Add to reformatted string with comma and space
                if [[ -n "$reformatted" ]]; then
                    reformatted="${reformatted}, ${part}"
                else
                    reformatted="${part}"
                fi
            fi
        done

        # Now set up completion based on the reformatted string
        if [[ -n "$reformatted" ]]; then
            local prefix="${reformatted}, "
            cur="$last_part"

            # Get already specified tags from the reformatted string
            local already_specified=()
            IFS=',' read -ra specified_tags <<< "$reformatted"
            for tag in "${specified_tags[@]}"; do
                tag="${tag# }"
                tag="${tag%% }"
                if [[ -n "$tag" ]]; then
                    already_specified+=("$tag")
                fi
            done

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

            # Generate completions
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

    # Get all already specified tags (from all previous words)
    local already_specified=()
    local i
    for ((i=2; i<=$COMP_CWORD; i++)); do
        local word="${COMP_WORDS[i]}"
        # Remove trailing comma and spaces
        word="${word%%,}"
        word="${word# }"
        word="${word%% }"

        # Skip empty words and the current word being completed
        if [[ -n "$word" ]] && [[ "$word" != "install" ]] && [[ $i -ne $COMP_CWORD ]]; then
            already_specified+=("$word")
        fi
    done

    # Also check current word for already typed tags (before the last comma)
    if [[ "$cur" == *,* ]]; then
        # Split by comma and add all but the last part
        local cur_prefix="${cur%%,*}"
        IFS=',' read -ra cur_tags <<< "$cur_prefix"
        for tag in "${cur_tags[@]}"; do
            # Trim spaces
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

    # Handle comma-separated tags with spaces
    if [[ "$cur" == *,* ]]; then
        # Get the prefix (everything before the last comma) and the partial tag after the last comma
        local prefix="${cur%%,*},"
        local partial="${cur##*,}"

        # Handle case where there's a space after the comma
        if [[ "$partial" == " "* ]]; then
            partial="${partial# }"  # Remove leading space
            prefix="${prefix} "     # Add space to prefix
        fi

        # Generate completions for the partial tag
        local matches=()
        for tag in "${available_tags[@]}"; do
            if [[ "$tag" == "$partial"* ]]; then
                # Add comma and space after the tag for easier chaining
                matches+=("${prefix}${tag}, ")
            fi
        done

        COMPREPLY=("${matches[@]}")

        # Disable space after completion since we're adding it ourselves
        compopt -o nospace 2>/dev/null
    elif [[ "$prev" == "," ]]; then
        # If previous char is a comma, complete with available tags
        COMPREPLY=($(compgen -W "${available_tags[*]}" -- "$cur"))

        # Add comma and space after each completion for chaining
        local i
        for i in "${!COMPREPLY[@]}"; do
            COMPREPLY[$i]="${COMPREPLY[$i]}, "
        done

        compopt -o nospace 2>/dev/null
    else
        # No comma in current word, complete normally with available tags
        COMPREPLY=($(compgen -W "${available_tags[*]}" -- "$cur"))

        # For space-separated completion, let bash add space automatically
        # For potential comma-separated, disable space so user can add comma
        if [[ ${#COMPREPLY[@]} -eq 1 ]]; then
            # Single match - user might want to add comma or space
            compopt -o nospace 2>/dev/null
        fi
    fi
}

_%s_completion() {
    local cur prev
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # List of main commands
    local commands="install list update version completion"

    # If we're completing the first argument (the command)
    if [ $COMP_CWORD -eq 1 ]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
        return 0
    fi

    # Check if 'install' command was used anywhere in the command line
    local has_install=0
    local i
    for ((i=1; i<COMP_CWORD; i++)); do
        if [[ "${COMP_WORDS[i]}" == "install" ]]; then
            has_install=1
            break
        fi
    done

    # If we have 'install' command, complete with tags
    if [[ $has_install -eq 1 ]] || [[ "$cur" == *,* ]]; then
        _%s_install_completion
        return 0
    fi
}

complete -F _%s_completion %s
`, cmdName, cmdName, formatTagsForBash(tags), cmdName, cmdName, cmdName, cmdName)

	// Write to file
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.WriteString(script); err != nil {
		return err
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

// generateBashCompletionAlias generates bash completion for the alias command (e.g., sb2)
func generateBashCompletionAlias(path string) error {
	return generateStaticBashCompletion(path, constants.CompletionAlias)
}

// generateZshCompletion generates zsh completion for the main 'sb' command
func generateZshCompletion(path string) error {
	return generateStaticZshCompletion(path, "sb")
}

// generateStaticZshCompletion creates a static zsh completion script with tags embedded
func generateStaticZshCompletion(path, cmdName string) error {
	// Load cache and get tags
	cacheInstance, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	// Get tags from cache
	tags := getCompletionTags(cacheInstance)
	if len(tags) == 0 {
		return fmt.Errorf("no tags found in cache - run 'sb list' first to populate the cache")
	}

	// Generate the static completion script
	script := fmt.Sprintf(`#compdef %s

# %s zsh completion script

_%s() {
    local -a tags commands
    local context state line

    # List of main commands
    commands=(
        'install:Install specified tags'
        'list:List available tags'
        'update:Update %s'
        'version:Show version'
        'completion:Generate completion script'
    )

    # Static list of available tags
    tags=(
%s
    )

    # First argument should be a command
    if [[ CURRENT -eq 2 ]]; then
        _describe -t commands '%s commands' commands
        return
    fi

    # If we're past the command and the command was 'install'
    if [[ ${words[2]} == "install" ]] && [[ CURRENT -ge 3 ]]; then
        _%s_install_tags
    fi
}

_%s_install_tags() {
    local cur_word="${words[CURRENT]}"
    local -a already_specified available_tags
    local tag specified

    # Function to extract all already specified tags from the command line
    _extract_specified_tags() {
        local word part
        already_specified=()

        # Check all previous words (skip '%s' and 'install')
        for ((i=3; i<CURRENT; i++)); do
            word="${words[i]}"
            # Handle both space-separated and comma-separated tags
            if [[ "$word" == *,* ]]; then
                # Split by comma
                for part in ${(s:,:)word}; do
                    # Trim spaces
                    part="${part## }"
                    part="${part%%%% }"
                    [[ -n "$part" ]] && already_specified+=("$part")
                done
            else
                # Single tag
                word="${word%%,}"
                word="${word## }"
                word="${word%%%% }"
                [[ -n "$word" ]] && already_specified+=("$word")
            fi
        done

        # Also process the current word if it contains commas
        if [[ "$cur_word" == *,* ]]; then
            # Get all but the last comma-separated part
            local prefix="${cur_word%%,*}"
            for part in ${(s:,:)prefix}; do
                part="${part## }"
                part="${part%%%% }"
                [[ -n "$part" ]] && already_specified+=("$part")
            done
        fi
    }

    # Extract already specified tags
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
        # Check if we need to reformat (has commas without proper spaces)
        local needs_reformat=0
        local test_word="$cur_word"

        # Check if there are commas not followed by spaces (except at the end)
        if [[ "$test_word" =~ ',[^ ]' ]] && [[ "$test_word" != *, ]]; then
            needs_reformat=1
        fi

        if [[ $needs_reformat -eq 1 ]]; then
            # Split and reformat with spaces
            local reformatted=""
            local parts=(${(s:,:)cur_word})
            local last_part=""

            for ((i=1; i<=$#parts; i++)); do
                local part="${parts[i]}"
                # Trim spaces
                part="${part## }"
                part="${part%%%% }"

                if [[ $i -eq $#parts ]]; then
                    # Last part (potentially incomplete)
                    last_part="$part"
                else
                    # Add to reformatted string
                    if [[ -n "$reformatted" ]]; then
                        reformatted="${reformatted}, ${part}"
                    else
                        reformatted="${part}"
                    fi
                fi
            done

            # Build completions with reformatted prefix
            if [[ -n "$reformatted" ]]; then
                local prefix="${reformatted}, "
                local -a matches

                for tag in $available_tags; do
                    if [[ "$tag" == ${last_part}* ]]; then
                        matches+=("${prefix}${tag}")
                    fi
                done

                if [[ ${#matches} -gt 0 ]]; then
                    # -U: Use the matches as-is, replacing the entire current word
                    # -Q: Don't quote special characters
                    # -S ', ': Add suffix for continuation
                    compadd -U -Q -S ', ' -- $matches
                    return
                fi
            fi
        else
            # Already properly formatted or ends with comma
            local prefix="${cur_word%%,*},"
            local partial="${cur_word##*,}"

            # Handle space after comma
            if [[ "$partial" == " "* ]]; then
                partial="${partial# }"
                prefix="${prefix} "
            else
                # No space after comma, add it
                prefix="${prefix} "
            fi

            # Generate matches
            local -a matches
            for tag in $available_tags; do
                if [[ "$tag" == ${partial}* ]]; then
                    matches+=("${prefix}${tag}")
                fi
            done

            if [[ ${#matches} -gt 0 ]]; then
                # Replace the entire word with properly formatted version
                compadd -U -Q -S ', ' -- $matches
                return
            fi
        fi
    else
        # No comma in current word
        # Complete and add ", " suffix to encourage proper formatting
        if [[ ${#available_tags} -gt 0 ]]; then
            local -a matching_tags
            for tag in $available_tags; do
                if [[ "$tag" == ${cur_word}* ]]; then
                    matching_tags+=("$tag")
                fi
            done

            if [[ ${#matching_tags} -gt 0 ]]; then
                # Add completions with ", " suffix to encourage comma-space format
                compadd -Q -S ', ' -- $matching_tags
            fi
        fi
    fi
}

# Special handling for zsh's menu completion
zstyle ':completion:*:*:%s:*:*' menu select

# Group the completions nicely
zstyle ':completion:*:descriptions' format '%%B%%d%%b'
zstyle ':completion:*:*:%s:*' group-name ''

# Don't sort the tags alphabetically - keep them in the order defined
zstyle ':completion:*:*:%s:*:*' sort false

# Register the completion function
compdef _%s %s
`, cmdName, cmdName, cmdName, cmdName, formatTagsForZsh(tags), cmdName, cmdName, cmdName, cmdName, cmdName, cmdName, cmdName, cmdName, cmdName)

	// Write to file
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.WriteString(script); err != nil {
		return err
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

// generateZshCompletionAlias generates zsh completion for the alias command (e.g., sb2)
func generateZshCompletionAlias(path string) error {
	return generateStaticZshCompletion(path, constants.CompletionAlias)
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
