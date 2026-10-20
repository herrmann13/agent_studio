package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/herrmann13/agent_studio/internal/application"
	"github.com/herrmann13/agent_studio/internal/domain"
)

const (
	escapeSequence        = "\033"
	clearScreenSequence   = "\033[H\033[J"
	boldSequence          = "\033[1m"
	resetSequence         = "\033[0m"
	selectedColor         = "\033[38;5;117m"
	mutedColor            = "\033[38;5;245m"
	errorColor            = "\033[38;5;203m"
	successColor          = "\033[38;5;114m"
	defaultInvocationMode = "automatic"
)

type dialog string

const (
	dialogNone       dialog = ""
	dialogMode       dialog = "mode"
	dialogCopy       dialog = "copy"
	dialogDelete     dialog = "delete"
	dialogProject    dialog = "project"
	dialogInstallURL dialog = "install-url"
	dialogInstallTo  dialog = "install-target"
)

type terminal struct {
	state string
}

func enterTerminal() (*terminal, error) {
	stateCommand := exec.Command("stty", "-g")
	stateCommand.Stdin = os.Stdin
	state, err := stateCommand.Output()
	if err != nil {
		return nil, fmt.Errorf("interactive terminal is required: %w", err)
	}
	rawCommand := exec.Command("stty", "raw", "-echo")
	rawCommand.Stdin = os.Stdin
	if err := rawCommand.Run(); err != nil {
		return nil, fmt.Errorf("enable terminal input: %w", err)
	}
	return &terminal{state: strings.TrimSpace(string(state))}, nil
}

func (t *terminal) restore() {
	if t != nil && t.state != "" {
		restoreCommand := exec.Command("stty", t.state)
		restoreCommand.Stdin = os.Stdin
		_ = restoreCommand.Run()
	}
	fmt.Print(escapeSequence + "[?25h" + escapeSequence + "[?1049l")
}

type model struct {
	service        *application.DiscoveryService
	workspace      domain.DiscoveryResult
	focusScopes    bool
	scopeIndex     int
	skillIndex     int
	dialog         dialog
	dialogIndex    int
	input          string
	installURL     string
	message        string
	messageIsError bool
}

func Run() error {
	service, err := application.DefaultDiscoveryService()
	if err != nil {
		return err
	}
	workspace, err := service.Discover()
	if err != nil {
		return err
	}
	terminal, err := enterTerminal()
	if err != nil {
		return err
	}
	defer terminal.restore()
	fmt.Print(escapeSequence + "[?1049h" + escapeSequence + "[?25l")

	app := model{service: service, workspace: workspace, focusScopes: true}
	reader := bufio.NewReader(os.Stdin)
	for {
		app.render()
		key, err := readKey(reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if app.handleKey(key) {
			return nil
		}
	}
}

func readKey(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadByte()
	if err != nil {
		return "", err
	}
	if value != 27 {
		return string(value), nil
	}
	// Setas chegam como uma sequência no mesmo read; um Esc isolado não traz bytes extras.
	if reader.Buffered() == 0 {
		return "esc", nil
	}
	next, err := reader.ReadByte()
	if err != nil || next != '[' {
		return "esc", nil
	}
	last, err := reader.ReadByte()
	if err != nil {
		return "esc", nil
	}
	switch last {
	case 'A':
		return "up", nil
	case 'B':
		return "down", nil
	case 'C':
		return "right", nil
	case 'D':
		return "left", nil
	default:
		return "esc", nil
	}
}

func (m *model) handleKey(key string) bool {
	if m.dialog != dialogNone {
		m.handleDialogKey(key)
		return false
	}
	if key == "q" || key == "\x03" {
		return true
	}
	switch key {
	case "\t", "left", "right":
		m.focusScopes = !m.focusScopes
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "r":
		m.refresh("Workspace refreshed.")
	case "m":
		if skill := m.selectedSkill(); skill != nil {
			m.dialog, m.dialogIndex = dialogMode, invocationModeIndex(skill.InvocationMode)
		}
	case "c":
		if m.selectedSkill() != nil && len(m.workspace.Scopes) > 1 {
			m.dialog, m.dialogIndex = dialogCopy, 0
		}
	case "d":
		if m.selectedSkill() != nil {
			m.dialog = dialogDelete
		}
	case "p":
		workingDirectory, err := os.Getwd()
		if err != nil {
			workingDirectory = ""
		}
		m.dialog, m.input = dialogProject, workingDirectory
	case "P":
		m.removeSelectedProject()
	case "i":
		if m.selectedScope() != nil {
			m.dialog, m.input = dialogInstallURL, ""
		}
	}
	return false
}

func (m *model) handleDialogKey(key string) {
	if key == "esc" {
		m.dialog, m.input = dialogNone, ""
		return
	}
	if m.dialog == dialogProject || m.dialog == dialogInstallURL {
		if key == "\r" || key == "\n" {
			m.submitInput()
			return
		}
		if key == "\x7f" || key == "\b" {
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return
		}
		if len(key) == 1 && key >= " " {
			m.input += key
		}
		return
	}
	if key == "up" || key == "k" {
		m.moveDialogSelection(-1)
		return
	}
	if key == "down" || key == "j" {
		m.moveDialogSelection(1)
		return
	}
	if key == "n" && m.dialog == dialogDelete {
		m.dialog = dialogNone
		return
	}
	if m.dialog == dialogDelete {
		if key == "y" {
			m.submitDialog()
		}
		return
	}
	if key == "\r" || key == "\n" {
		m.submitDialog()
	}
}

func (m *model) submitInput() {
	switch m.dialog {
	case dialogProject:
		path := strings.TrimSpace(m.input)
		if path == "" {
			m.setError("Project path cannot be empty.")
			return
		}
		workspace, err := m.service.AddProject(path)
		if err != nil {
			m.setError(err.Error())
			return
		}
		m.workspace, m.dialog, m.input = workspace, dialogNone, ""
		m.setMessage("Project added.")
	case dialogInstallURL:
		if strings.TrimSpace(m.input) == "" {
			m.setError("Skill URL cannot be empty.")
			return
		}
		m.installURL, m.dialog, m.dialogIndex = m.input, dialogInstallTo, m.scopeIndex
	}
}

func (m *model) submitDialog() {
	skill := m.selectedSkill()
	switch m.dialog {
	case dialogMode:
		if skill == nil {
			return
		}
		workspace, err := m.service.SetSkillInvocationMode(skill.Path, invocationModes()[m.dialogIndex])
		m.applyWorkspace(workspace, err, "Skill mode updated.")
	case dialogCopy:
		if skill == nil || len(m.workspace.Scopes) == 0 {
			return
		}
		target := m.workspace.Scopes[m.dialogIndex]
		workspace, err := m.service.CopySkill(skill.Path, target.ID)
		m.applyWorkspace(workspace, err, "Skill copied.")
	case dialogDelete:
		if skill == nil {
			return
		}
		workspace, err := m.service.DeleteSkill(skill.Path)
		m.applyWorkspace(workspace, err, "Skill deleted.")
	case dialogInstallTo:
		if len(m.workspace.Scopes) == 0 {
			return
		}
		result, err := m.service.InstallSkillFromURL(m.installURL, m.workspace.Scopes[m.dialogIndex].ID)
		if err != nil {
			m.setError(err.Error())
			return
		}
		m.workspace, m.dialog, m.installURL = result.Workspace, dialogNone, ""
		m.setMessage("Skill installed via " + result.Method + ".")
	}
}

func (m *model) applyWorkspace(workspace domain.DiscoveryResult, err error, successMessage string) {
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.workspace, m.dialog = workspace, dialogNone
	m.normalizeSelection()
	m.setMessage(successMessage)
}

func (m *model) refresh(message string) {
	workspace, err := m.service.Discover()
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.workspace = workspace
	m.normalizeSelection()
	m.setMessage(message)
}

func (m *model) removeSelectedProject() {
	scope := m.selectedScope()
	if scope == nil || scope.Kind != "project" {
		m.setError("Select a project scope to remove it.")
		return
	}
	projectID := strings.TrimPrefix(scope.ID, "project:")
	workspace, err := m.service.RemoveProject(projectID)
	m.applyWorkspace(workspace, err, "Project removed.")
}

func (m *model) moveSelection(offset int) {
	if m.focusScopes {
		m.scopeIndex = boundedIndex(m.scopeIndex+offset, len(m.workspace.Scopes))
		m.skillIndex = 0
		return
	}
	m.skillIndex = boundedIndex(m.skillIndex+offset, len(m.skillsForSelectedScope()))
}

func (m *model) moveDialogSelection(offset int) {
	limit := 0
	switch m.dialog {
	case dialogMode:
		limit = len(invocationModes())
	case dialogCopy, dialogInstallTo:
		limit = len(m.workspace.Scopes)
	}
	m.dialogIndex = boundedIndex(m.dialogIndex+offset, limit)
}

func boundedIndex(index, length int) int {
	if length == 0 {
		return 0
	}
	if index < 0 {
		return length - 1
	}
	return index % length
}

func (m *model) normalizeSelection() {
	m.scopeIndex = boundedIndex(m.scopeIndex, len(m.workspace.Scopes))
	m.skillIndex = boundedIndex(m.skillIndex, len(m.skillsForSelectedScope()))
}

func (m *model) selectedScope() *domain.Scope {
	if len(m.workspace.Scopes) == 0 {
		return nil
	}
	return &m.workspace.Scopes[m.scopeIndex]
}

func (m *model) skillsForSelectedScope() []domain.Skill {
	scope := m.selectedScope()
	if scope == nil {
		return nil
	}
	skills := make([]domain.Skill, 0)
	for _, skill := range m.workspace.Skills {
		if skill.ScopeID == scope.ID {
			skills = append(skills, skill)
		}
	}
	return skills
}

func (m *model) selectedSkill() *domain.Skill {
	skills := m.skillsForSelectedScope()
	if len(skills) == 0 {
		return nil
	}
	return &skills[m.skillIndex]
}

func (m *model) setMessage(message string) {
	m.message, m.messageIsError = message, false
}

func (m *model) setError(message string) {
	m.message, m.messageIsError = message, true
}

func invocationModes() []string {
	return []string{"automatic", "always", "explicit", "disabled"}
}

func invocationModeIndex(mode string) int {
	for index, candidate := range invocationModes() {
		if candidate == mode {
			return index
		}
	}
	return 0
}
