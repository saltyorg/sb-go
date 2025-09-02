package cmd

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/saltyorg/sb-go/constants"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/pbkdf2"
)

var (
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle  = focusedStyle
	noStyle      = lipgloss.NewStyle()
	helpStyle    = blurredStyle.Foreground(lipgloss.Color("240"))

	focusedButton = focusedStyle.Render("[ Submit ]")
	blurredButton = fmt.Sprintf("[ %s ]", blurredStyle.Render("Submit"))
)

// restoreModel holds the state of the Bubble Tea UI.
type restoreModel struct {
	focusIndex int
	inputs     []textinput.Model
	cursorMode cursor.Mode
	err        error
	user       string
	password   string
	submitted  bool
}

func initialRestoreModel() *restoreModel {
	m := &restoreModel{
		inputs:    make([]textinput.Model, 3),
		submitted: false,
	}

	var t textinput.Model
	for i := range m.inputs {
		t = textinput.New()
		t.Cursor.Style = cursorStyle
		t.CharLimit = 64

		switch i {
		case 0:
			t.Placeholder = "Username"
			t.Prompt = "Username: "
			t.Focus()
			t.PromptStyle = focusedStyle
			t.TextStyle = focusedStyle
		case 1:
			t.Placeholder = "Password"
			t.Prompt = "Password: "
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		case 2:
			t.Placeholder = "Confirm Password"
			t.Prompt = "Password: "
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}

		m.inputs[i] = t
	}

	return m
}

func (m *restoreModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *restoreModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		// Set focus to next input
		case "tab", "shift+tab", "enter", "up", "down":
			s := msg.String()

			// Did the user press enter while the submit button was focused?
			if s == "enter" && m.focusIndex == len(m.inputs) {
				// Validate a password match before proceeding.
				if m.inputs[1].Value() == m.inputs[2].Value() && m.inputs[1].Value() != "" {
					// Store values before quitting
					m.user = m.inputs[0].Value()
					m.password = m.inputs[1].Value()
					m.submitted = true
					m.err = nil
					return m, tea.Quit

				} else {
					// Set a validation error.
					m.err = errors.New("passwords do not match or are empty")
					return m, nil
				}
			}

			// Cycle indexes
			if s == "up" || s == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}

			if m.focusIndex > len(m.inputs) {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs)
			}

			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i < len(m.inputs); i++ {
				if i == m.focusIndex {
					// Set focused state
					cmds[i] = m.inputs[i].Focus()
					m.inputs[i].PromptStyle = focusedStyle
					m.inputs[i].TextStyle = focusedStyle
					continue
				}
				// Remove the focused state
				m.inputs[i].Blur()
				m.inputs[i].PromptStyle = noStyle
				m.inputs[i].TextStyle = noStyle
			}

			return m, tea.Batch(cmds...)
		}
	}

	// Handle character input and blinking
	cmd := m.updateInputs(msg)

	// Clear the error if the passwords now match (after typing in fields)
	if m.inputs[1].Value() == m.inputs[2].Value() && m.inputs[1].Value() != "" {
		m.err = nil
	}

	return m, cmd
}

func (m *restoreModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))

	// Only text inputs with Focus() set will respond, so it's safe to simply
	// update all of them here without any further logic.
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}

	return tea.Batch(cmds...)
}

func (m *restoreModel) View() string {
	var b strings.Builder

	for i := range m.inputs {
		b.WriteString(m.inputs[i].View())
		if i < len(m.inputs)-1 {
			b.WriteRune('\n')
		}
	}

	button := &blurredButton
	if m.focusIndex == len(m.inputs) {
		button = &focusedButton
	}
	fmt.Fprintf(&b, "\n\n%s\n\n", *button)

	// Display error, if any
	if m.err != nil {
		b.WriteString(helpStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteRune('\n')
	}
	//Remove the cursor mode section
	return b.String()
}

// restoreCmd represents the restore command
var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Fetches and decrypts files based on username and password",
	Long: `Fetches encrypted files from a remote URL, decrypts them, and places them in the Saltbox directory.
The restore URL defaults to "crs.saltbox.dev".  A password will be prompted for twice.`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		p := tea.NewProgram(initialRestoreModel(), tea.WithOutput(os.Stdout), tea.WithAltScreen())

		m, err := p.Run()
		if err != nil {
			fmt.Printf("could not start program: %s\n", err)
			os.Exit(1)
		}

		// Assert the model back to our model and check the submitted flag.
		if finalModel, ok := m.(*restoreModel); ok {
			if finalModel.submitted {
				// Form was submitted, proceed with restore.
				user := finalModel.user
				password := finalModel.password
				restoreURL := "https://crs.saltbox.dev"

				dir := constants.SaltboxRepoPath
				folder := filepath.Join(os.TempDir(), "saltbox_restore")
				verbose, _ := cmd.Flags().GetBool("verbose")

				successfulDownloads, err := validateAndRestore(user, password, restoreURL, dir, folder, verbose)
				if err != nil {
					fmt.Println("Error:", err)
					if verbose {
						fmt.Printf("DEBUG: Underlying error: %v\n", err)
					}
					os.Exit(1)
				}

				if successfulDownloads == 0 {
					fmt.Println("Restore process failed: No files were downloaded or decrypted.")
					os.Exit(1)
				} else {
					fmt.Printf("Restore process completed: %d files successfully restored.\n", successfulDownloads)
				}

			} else {
				// User exited without submitting, exit gracefully.
				fmt.Println("Restore cancelled.")
				os.Exit(0) // Exit with code 0 for a clean exit.
			}
		} else {
			fmt.Println("Error: Could not retrieve values from the UI.")
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
}

func validateAndRestore(user, password, restoreURL, dir, folder string, verbose bool) (int, error) {
	files := []string{"accounts.yml", "adv_settings.yml", "backup_config.yml", "hetzner_vlan.yml", "localhost.yml", "motd.yml", "providers.yml", "rclone.conf", "settings.yml"}
	if verbose {
		fmt.Printf("DEBUG: Creating temporary folder: %s\n", folder)
	}
	if err := os.MkdirAll(folder, 0700); err != nil {
		return 0, fmt.Errorf("failed to create temporary folder %s: %w", folder, err)
	}
	if verbose {
		fmt.Printf("DEBUG: Temp folder created\n")
	}
	defer os.RemoveAll(folder) // Clean up the temp folder afterward

	if verbose {
		fmt.Printf("DEBUG: Creating restore folder: %s\n", dir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create restore folder %s: %w", dir, err)
	}

	userHash := fmt.Sprintf("%x", sha1.Sum([]byte(user)))
	if verbose {
		fmt.Println("User Hash:", userHash)
	}

	fmt.Printf("Fetching files from %s...\n", restoreURL)
	successfulDownloads := 0
	for _, file := range files {
		fmt.Printf("%-20.20s", file)
		url := fmt.Sprintf("%s/load/%s/%s", restoreURL, userHash, file)
		if verbose {
			fmt.Printf("DEBUG: Fetching URL: %s\n", url)
		}

		if !validateURL(url, verbose) {
			fmt.Println(" [IGNORED]")
			continue
		}

		outFile := filepath.Join(folder, file+".enc")
		if verbose {
			fmt.Printf("DEBUG: Downloading to: %s\n", outFile)
		}
		if err := downloadFile(url, outFile); err != nil {
			fmt.Println(" [FAIL]")
			// Don't return immediately, continue to the next file
			continue
		}

		header := make([]byte, 10)
		f, err := os.Open(outFile)
		if err != nil {
			fmt.Println(" [FAIL]")
			// Don't return, continue to the next file
			continue
		}
		n, err := f.Read(header)
		f.Close()
		if err != nil && err != io.EOF {
			fmt.Println(" [FAIL]")
			//Don't return, continue
			continue
		}
		if verbose {
			fmt.Printf("DEBUG: Read %d bytes for header. Header: %s\n", n, string(header))
		}
		if strings.Contains(string(header), "Salted") {
			fmt.Println(" [DONE]")
			// Download was ok. Don't increment here.
		} else {
			fmt.Println(" [FAIL]")
			// Header not found.  Don't return; try the next file
			continue
		}

		encryptedFilePath := filepath.Join(folder, file+".enc")
		decryptedFilePath := filepath.Join(folder, file)

		if _, err := os.Stat(encryptedFilePath); err != nil {
			if verbose {
				fmt.Printf("DEBUG: Encrypted file does not exist, skipping: %s\n", encryptedFilePath)
			}
			continue
		}
		// Don't print here, wait for a decryption result.

		if err := decryptFile(encryptedFilePath, decryptedFilePath, password, verbose); err != nil {
			// Decryption failed. Print fail and continue to the next file.
			fmt.Printf("%-20.20s [FAIL]\n", file)
			var paddingErr *paddingError
			if errors.As(err, &paddingErr) {
				fmt.Println("Decryption failed. This likely means the password was incorrect")
			} else {
				fmt.Printf("  (Technical error: %v)\n", err) // More detailed error
			}
			continue // Continue to the next file
		}

		if verbose {
			fmt.Printf("DEBUG: Removing encrypted file: %s\n", encryptedFilePath)
		}
		err = os.Remove(encryptedFilePath)
		if err != nil {
			fmt.Printf("%-20.20s [FAIL] - Could not remove encrypted file: %v\n", file, err)
			continue
		}
		fmt.Printf("%-20.20s [DONE]\n", file)
		successfulDownloads++

		sourcePath := filepath.Join(folder, file)
		destPath := filepath.Join(dir, file)
		if verbose {
			fmt.Printf("DEBUG: Source Path: %s\n", sourcePath)
			fmt.Printf("DEBUG: Destination Path: %s\n", destPath)
		}

		if _, err := os.Stat(sourcePath); err != nil {
			if verbose {
				fmt.Printf("DEBUG: Source file does not exist, skipping move: %s\n", sourcePath)
			}
			continue // Continue to the next file
		}

		if err := os.Rename(sourcePath, destPath); err != nil {
			fmt.Printf("Failed to move %s: [FAIL]\n", file)
			continue
		}
	}
	return successfulDownloads, nil
}

func validateURL(url string, verbose bool) bool {
	if verbose {
		fmt.Printf("DEBUG: Validating URL: %s\n", url)
	}
	resp, err := http.Head(url)
	if err != nil {
		if verbose {
			fmt.Printf("DEBUG: URL validation failed: %v\n", err)
		}
		return false
	}
	defer resp.Body.Close()
	if verbose {
		fmt.Printf("DEBUG: URL validation status code: %d\n", resp.StatusCode)
	}
	return resp.StatusCode == http.StatusOK
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write to file during download: %v", err)
	}
	return err
}

// Custom error type for padding errors
type paddingError struct {
	err error
}

func (e *paddingError) Error() string {
	return e.err.Error()
}
func (e *paddingError) Unwrap() error {
	return e.err
}

func decryptFile(inputFile, outputFile, password string, verbose bool) error {
	if verbose {
		fmt.Printf("DEBUG: Decrypting %s to %s\n", inputFile, outputFile)
	}
	ciphertext, err := os.ReadFile(inputFile)
	if err != nil {
		return err
	}

	if len(ciphertext) < 16 || string(ciphertext[:8]) != "Salted__" {
		return fmt.Errorf("invalid ciphertext format (missing 'Salted__' prefix)")
	}

	salt := ciphertext[8:16]
	ciphertext = ciphertext[16:]
	if verbose {
		fmt.Printf("DEBUG: Salt: %x\n", salt)
	}
	key, iv := deriveKeyAndIV([]byte(password), salt, verbose)
	if verbose {
		fmt.Printf("DEBUG: Key: %x, IV: %x\n", key, iv)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	cbc := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	cbc.CryptBlocks(plaintext, ciphertext)

	paddingLen := int(plaintext[len(plaintext)-1])
	if verbose {
		fmt.Printf("DEBUG: Padding Size Before Check: %d\n", paddingLen)
	}
	if paddingLen > aes.BlockSize || paddingLen <= 0 {
		// Return a custom padding error
		return &paddingError{fmt.Errorf("invalid padding size: %d", paddingLen)}

	}

	for i := len(plaintext) - paddingLen; i < len(plaintext); i++ {
		if plaintext[i] != byte(paddingLen) {
			return &paddingError{fmt.Errorf("invalid padding")}
		}
	}

	plaintext = plaintext[:len(plaintext)-paddingLen]
	if verbose {
		fmt.Printf("DEBUG: Plaintext length after unpadding: %d\n", len(plaintext))
	}
	return os.WriteFile(outputFile, plaintext, 0644)
}

func deriveKeyAndIV(password, salt []byte, verbose bool) ([]byte, []byte) {
	if verbose {
		fmt.Printf("DEBUG: Deriving key and IV. Password length: %d, Salt: %x\n", len(password), salt)
	}
	iterations := 10000
	keySize := 48 // 32 for key, 16 for IV
	key := pbkdf2.Key(password, salt, iterations, keySize, sha256.New)
	return key[:32], key[32:]
}
