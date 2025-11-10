package announcements

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"
	"gopkg.in/yaml.v3"
)

type Migration struct {
	Required bool   `yaml:"required"`
	Tag      string `yaml:"tag"`
}

type Announcement struct {
	Date           string    `yaml:"date"`
	Title          string    `yaml:"title"`
	Migration      Migration `yaml:"migration"`
	Message        string    `yaml:"message"`
	RequiredFolder string    `yaml:"required_folder,omitempty"`
	RequiredFile   string    `yaml:"required_file,omitempty"`
}

type AnnouncementFile struct {
	Announcements []Announcement `yaml:"announcements"`
}

type AnnouncementDiff struct {
	RepoName         string
	RepoPath         string
	NewAnnouncements []Announcement
}

type MigrationRequest struct {
	RepoName string
	RepoPath string
	Tag      string
}

// announcementViewer is a Bubble Tea model for displaying announcements
type announcementViewer struct {
	viewport        viewport.Model
	currentIndex    int
	announcements   []announcementItem
	renderedContent map[int]string // Pre-rendered markdown content by index
	ready           bool
	err             error
	renderer        *glamour.TermRenderer // Stored for potential future use
}

type announcementItem struct {
	announcement Announcement
	repoName     string
}

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render

// LoadAllAnnouncementFiles loads announcements from both Saltbox and Sandbox repositories
// Returns saltboxFile, sandboxFile, error
func LoadAllAnnouncementFiles() (*AnnouncementFile, *AnnouncementFile, error) {
	saltboxFile, err := LoadSingleAnnouncementFile(GetAnnouncementFilePath(constants.SaltboxRepoPath))
	if err != nil {
		return nil, nil, fmt.Errorf("error loading Saltbox announcements: %w", err)
	}

	sandboxFile, err := LoadSingleAnnouncementFile(GetAnnouncementFilePath(constants.SandboxRepoPath))
	if err != nil {
		return nil, nil, fmt.Errorf("error loading Sandbox announcements: %w", err)
	}

	return saltboxFile, sandboxFile, nil
}

// LoadSingleAnnouncementFile loads announcements from a YAML file
// Returns nil if file doesn't exist, empty AnnouncementFile if file exists but has no announcements
func LoadSingleAnnouncementFile(filePath string) (*AnnouncementFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return nil to indicate file doesn't exist (initial case)
			return nil, nil
		}
		return nil, fmt.Errorf("error reading announcement file: %w", err)
	}

	var announcementFile AnnouncementFile
	err = yaml.Unmarshal(data, &announcementFile)
	if err != nil {
		return nil, fmt.Errorf("error parsing announcement YAML: %w", err)
	}

	// Ensure announcements slice is not nil
	if announcementFile.Announcements == nil {
		announcementFile.Announcements = []Announcement{}
	}

	return &announcementFile, nil
}

// CheckForNewAnnouncementsAllRepos checks for new announcements in both Saltbox and Sandbox repositories
func CheckForNewAnnouncementsAllRepos(saltboxBefore, saltboxAfter, sandboxBefore, sandboxAfter *AnnouncementFile) []*AnnouncementDiff {
	saltboxDiff := CheckSingleRepoAnnouncements("Saltbox", constants.SaltboxRepoPath, saltboxBefore, saltboxAfter)
	sandboxDiff := CheckSingleRepoAnnouncements("Sandbox", constants.SandboxRepoPath, sandboxBefore, sandboxAfter)

	return []*AnnouncementDiff{saltboxDiff, sandboxDiff}
}

// CheckSingleRepoAnnouncements compares before and after announcement files and returns new announcements for a single repo
func CheckSingleRepoAnnouncements(repoName, repoPath string, beforeFile, afterFile *AnnouncementFile) *AnnouncementDiff {
	// If there's no after file, there are no new announcements
	if afterFile == nil {
		return &AnnouncementDiff{
			RepoName:         repoName,
			RepoPath:         repoPath,
			NewAnnouncements: []Announcement{},
		}
	}

	// If there was no before file (initial case), all announcements in after file are new
	if beforeFile == nil || len(beforeFile.Announcements) == 0 {
		var newAnnouncements []Announcement
		for _, announcement := range afterFile.Announcements {
			if shouldShowAnnouncement(announcement) {
				newAnnouncements = append(newAnnouncements, announcement)
			}
		}

		// Sort announcements by date (oldest to newest)
		sortAnnouncementsByDate(newAnnouncements)

		return &AnnouncementDiff{
			RepoName:         repoName,
			RepoPath:         repoPath,
			NewAnnouncements: newAnnouncements,
		}
	}

	// Create a map of existing announcements based on date+title for quick lookup
	existingMap := make(map[string]bool)
	for _, announcement := range beforeFile.Announcements {
		key := announcement.Date + "|" + announcement.Title
		existingMap[key] = true
	}

	// Find new announcements
	var newAnnouncements []Announcement
	for _, announcement := range afterFile.Announcements {
		key := announcement.Date + "|" + announcement.Title
		if !existingMap[key] && shouldShowAnnouncement(announcement) {
			newAnnouncements = append(newAnnouncements, announcement)
		}
	}

	// Sort announcements by date (oldest to newest)
	sortAnnouncementsByDate(newAnnouncements)

	return &AnnouncementDiff{
		RepoName:         repoName,
		RepoPath:         repoPath,
		NewAnnouncements: newAnnouncements,
	}
}

// sortAnnouncementsByDate sorts announcements by date in ascending order (oldest to newest)
func sortAnnouncementsByDate(announcements []Announcement) {
	sort.Slice(announcements, func(i, j int) bool {
		return announcements[i].Date < announcements[j].Date
	})
}

// shouldShowAnnouncement checks if the announcement's path conditions are satisfied.
// Returns true if:
// - required_folder is not set OR the specified folder exists and is a directory
// - required_file is not set OR the specified file exists and is a regular file
// If both are set, BOTH conditions must be true.
func shouldShowAnnouncement(announcement Announcement) bool {
	// Check required_folder if specified
	if announcement.RequiredFolder != "" {
		info, err := os.Stat(announcement.RequiredFolder)
		if os.IsNotExist(err) {
			return false
		}
		// Verify it's actually a directory
		if err == nil && !info.IsDir() {
			return false
		}
	}

	// Check required_file if specified
	if announcement.RequiredFile != "" {
		info, err := os.Stat(announcement.RequiredFile)
		if os.IsNotExist(err) {
			return false
		}
		// Verify it's actually a file (not a directory)
		if err == nil && info.IsDir() {
			return false
		}
	}

	return true
}

// GetAnnouncementFilePath returns the path to the announcements.yml file for a repo
func GetAnnouncementFilePath(repoPath string) string {
	return filepath.Join(repoPath, "announcements.yml")
}

// newAnnouncementViewer creates a new announcement viewer
func newAnnouncementViewer(announcements []announcementItem) *announcementViewer {
	return &announcementViewer{
		viewport:        viewport.New(0, 0), // Will be sized on first WindowSizeMsg
		currentIndex:    0,
		announcements:   announcements,
		renderedContent: make(map[int]string),
		ready:           false,
		renderer:        nil,
	}
}

func (av *announcementViewer) Init() tea.Cmd {
	// Wait for WindowSizeMsg before rendering
	return nil
}

func (av *announcementViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !av.ready {
			// First time setup with fixed dimensions (original 78x20)
			const width = 78
			const height = 20

			av.viewport = viewport.New(width, height)
			av.viewport.Style = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				PaddingRight(2)
			av.ready = true

			// Display the first pre-rendered announcement immediately
			if content, ok := av.renderedContent[av.currentIndex]; ok {
				av.viewport.SetContent(content)
				av.viewport.GotoTop()
			}
		}
		// Ignore window resizes - we keep the fixed 78x20 dimensions
		return av, nil

	case tea.KeyMsg:
		// Global quit keys
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return av, tea.Quit
		case "right", "l":
			// Move to next announcement (all pre-rendered - instant!)
			if av.currentIndex < len(av.announcements)-1 {
				av.currentIndex++
				if content, ok := av.renderedContent[av.currentIndex]; ok {
					av.viewport.SetContent(content)
					av.viewport.GotoTop()
				}
			}
			return av, nil
		case "left", "h":
			// Move to previous announcement (all pre-rendered - instant!)
			if av.currentIndex > 0 {
				av.currentIndex--
				if content, ok := av.renderedContent[av.currentIndex]; ok {
					av.viewport.SetContent(content)
					av.viewport.GotoTop()
				}
			}
			return av, nil
		default:
			// Delegate viewport scrolling to the viewport model
			var cmd tea.Cmd
			av.viewport, cmd = av.viewport.Update(msg)
			return av, cmd
		}

	default:
		// Pass other messages to viewport
		var cmd tea.Cmd
		av.viewport, cmd = av.viewport.Update(msg)
		return av, cmd
	}
}

func (av *announcementViewer) View() string {
	if av.err != nil {
		return fmt.Sprintf("Error rendering announcement: %v\n", av.err)
	}

	if !av.ready {
		return "Initializing..."
	}

	// Compose view using pre-rendered viewport content
	return av.viewport.View() + av.helpView()
}

func (av *announcementViewer) helpView() string {
	if len(av.announcements) <= 1 {
		return helpStyle("\n  ↑/↓: Navigate • q: Quit\n")
	}

	current := av.currentIndex + 1
	total := len(av.announcements)
	position := fmt.Sprintf("(%d/%d)", current, total)

	var navigation []string
	if av.currentIndex > 0 {
		navigation = append(navigation, "←/h: Previous")
	}
	if av.currentIndex < len(av.announcements)-1 {
		navigation = append(navigation, "→/l: Next")
	}

	navText := strings.Join(navigation, " • ")
	if len(navigation) > 0 {
		return helpStyle(fmt.Sprintf("\n  ↑/↓: Navigate • %s %s • q: Quit\n", navText, position))
	}
	return helpStyle(fmt.Sprintf("\n  ↑/↓: Navigate • q: Quit %s\n", position))
}

// DisplayAnnouncements displays new announcements using Glamour and Bubble Tea
func DisplayAnnouncements(diffs []*AnnouncementDiff) error {
	// Collect all announcements
	var allAnnouncements []announcementItem

	for _, diff := range diffs {
		for _, announcement := range diff.NewAnnouncements {
			allAnnouncements = append(allAnnouncements, announcementItem{
				announcement: announcement,
				repoName:     diff.RepoName,
			})
		}
	}

	if len(allAnnouncements) == 0 {
		return nil
	}

	// Info message before displaying announcements
	if err := spinners.RunInfoSpinner("Displaying new announcements"); err != nil {
		return err
	}

	// Pre-render all announcements BEFORE starting Bubbletea
	// This avoids any async complexity and matches the fast plain-text version
	renderedContent := make(map[int]string)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(78), // Use a reasonable default width
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %w", err)
	}

	for i, item := range allAnnouncements {
		parsedDate, err := time.Parse("2006-01-02", item.announcement.Date)
		var formattedDate string
		if err != nil {
			formattedDate = item.announcement.Date
		} else {
			formattedDate = parsedDate.Format("January 2, 2006")
		}

		content := fmt.Sprintf("# %s Announcement - %s\n%s", item.repoName, formattedDate, item.announcement.Message)
		rendered, err := renderer.Render(content)
		if err != nil {
			return fmt.Errorf("failed to render announcement: %w", err)
		}
		renderedContent[i] = rendered
	}

	// Create viewer with pre-rendered content
	viewer := newAnnouncementViewer(allAnnouncements)
	viewer.renderedContent = renderedContent
	viewer.renderer = renderer // Store for potential resizes

	// Run with alt screen - use stdin/stdout explicitly to avoid TTY issues
	p := tea.NewProgram(
		viewer,
		tea.WithAltScreen(),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run announcement viewer: %w", err)
	}

	// Check if there was an error during rendering
	if viewer.err != nil {
		return fmt.Errorf("error during announcement display: %w", viewer.err)
	}

	return nil
}

// PromptForMigrations prompts the user for migration approvals and returns migration requests
func PromptForMigrations(diffs []*AnnouncementDiff) ([]MigrationRequest, error) {
	var migrationRequests []MigrationRequest
	var hasMigrations bool

	// Check if there are any migrations to prompt for
	for _, diff := range diffs {
		for _, announcement := range diff.NewAnnouncements {
			if announcement.Migration.Required && announcement.Migration.Tag != "" {
				hasMigrations = true
				break
			}
		}
		if hasMigrations {
			break
		}
	}

	if !hasMigrations {
		return migrationRequests, nil
	}

	scanner := bufio.NewScanner(os.Stdin)

	for _, diff := range diffs {
		for _, announcement := range diff.NewAnnouncements {
			if announcement.Migration.Required && announcement.Migration.Tag != "" {
				// Prompt for migration approval with validation
				prompt := fmt.Sprintf("Run migration '%s' for %s repository? (y/n): ", announcement.Migration.Tag, diff.RepoName)

				var response string
				for {
					fmt.Print(prompt)
					scanner.Scan()
					response = strings.TrimSpace(strings.ToLower(scanner.Text()))

					// Validate input - require explicit y/yes/n/no
					if response == "y" || response == "yes" || response == "n" || response == "no" {
						break
					}

					// Show error for invalid input
					fmt.Println("Invalid input. Please enter 'y' (yes) or 'n' (no).")
				}

				if response == "y" || response == "yes" {
					migrationRequests = append(migrationRequests, MigrationRequest{
						RepoName: diff.RepoName,
						RepoPath: diff.RepoPath,
						Tag:      announcement.Migration.Tag,
					})
				}
			}
		}
	}

	return migrationRequests, nil
}

// ExecuteMigrations runs the requested migration playbook tags.
// It accepts a context parameter for proper cancellation support.
func ExecuteMigrations(ctx context.Context, migrationRequests []MigrationRequest) error {
	if len(migrationRequests) == 0 {
		return nil
	}

	if err := spinners.RunInfoSpinner("Starting migration execution"); err != nil {
		return err
	}

	for _, migration := range migrationRequests {
		// Info message before running each migration
		migrationMsg := fmt.Sprintf("Running migration '%s' for %s repository", migration.Tag, migration.RepoName)
		if err := spinners.RunInfoSpinner(migrationMsg); err != nil {
			return err
		}

		// Determine the correct playbook path based on repository
		var playbookPath string
		switch migration.RepoPath {
		case constants.SaltboxRepoPath:
			playbookPath = constants.SaltboxPlaybookPath()
		case constants.SandboxRepoPath:
			playbookPath = constants.SandboxPlaybookPath()
		case constants.SaltboxModRepoPath:
			playbookPath = constants.SaltboxModPlaybookPath()
		default:
			return fmt.Errorf("unknown repository path: %s", migration.RepoPath)
		}

		// Run the ansible playbook with the migration tag using the provided context
		extraArgs := []string{"--tags", migration.Tag}
		err := ansible.RunAnsiblePlaybook(ctx, migration.RepoPath, playbookPath, constants.AnsiblePlaybookBinaryPath, extraArgs, true)
		if err != nil {
			return fmt.Errorf("failed to execute migration '%s' for %s repository: %w", migration.Tag, migration.RepoName, err)
		}
	}

	if err := spinners.RunInfoSpinner("All migrations completed successfully"); err != nil {
		return err
	}

	return nil
}
