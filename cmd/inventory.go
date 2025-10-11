package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
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
	cmd := exec.Command(editor, filePath)
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
