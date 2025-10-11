package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"

	"github.com/spf13/cobra"
)

// inventoryCmd represents the inventory command
var inventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Manage Saltbox inventory",
	Long:  `Manage Saltbox inventory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleInventory()
	},
}

func init() {
	rootCmd.AddCommand(inventoryCmd)
}

func handleInventory() error {
	defaultEditor := "nano"
	approvedEditors := []string{"nano", "vim", "vi", "emacs", "gedit", "code", "micro"}

	if _, err := os.Stat(constants.SaltboxInventoryPath); os.IsNotExist(err) {
		return fmt.Errorf("error: the inventory file 'localhost.yml' does not yet exist")
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = defaultEditor
	}

	isApproved := slices.Contains(approvedEditors, editor)

	if !isApproved {
		if editor == defaultEditor {
			return runEditor(defaultEditor, constants.SaltboxInventoryPath)
		}

		fmt.Printf("The EDITOR variable is set to an unrecognized value: %s\n", editor)
		confirm, err := confirmInput("Are you sure you want to use it to edit the file? (y/N) ")
		if err != nil {
			return err
		}

		if confirm {
			return runEditor(editor, constants.SaltboxInventoryPath)
		}

		fmt.Printf("Using default editor: %s\n", defaultEditor)
		return runEditor(defaultEditor, constants.SaltboxInventoryPath)
	}

	return runEditor(editor, constants.SaltboxInventoryPath)
}

func runEditor(editor, filePath string) error {
	// Validate and sanitize editor command
	// Split on whitespace to get the base command
	editorParts := strings.Fields(editor)
	if len(editorParts) == 0 {
		return fmt.Errorf("invalid editor command")
	}

	// Get the absolute path of the editor executable
	editorPath, err := exec.LookPath(editorParts[0])
	if err != nil {
		// If not in PATH, check if it's an absolute path
		if filepath.IsAbs(editorParts[0]) {
			editorPath = editorParts[0]
		} else {
			return fmt.Errorf("editor '%s' not found in PATH", editorParts[0])
		}
	}

	// Construct command with validated editor and additional args if any
	var cmd *exec.Cmd
	if len(editorParts) > 1 {
		// Include any additional arguments from editor variable
		args := append(editorParts[1:], filePath)
		cmd = exec.Command(editorPath, args...)
	} else {
		cmd = exec.Command(editorPath, filePath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func confirmInput(prompt string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y", nil
}
