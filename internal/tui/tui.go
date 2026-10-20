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
	// skills.sh search: type a query, pick a result, then pick the scope.
	dialogRegistrySearch  dialog = "registry-search"
	dialogRegistryResults dialog = "registry-results"
	dialogRegistryTarget  dialog = "registry-target"
	// dialogAgents chooses which installed agents are tracked (space toggles).
	dialogAgents dialog = "agents"
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
	registry       []domain.RegistrySkill
	registryChoice int
	message        string
	messageIsError bool
	// busy is set while network work (skills.sh search, installs) runs in the
	// background; keys are ignored meanwhile so a stray Enter cannot act on stale
	// results. The finished work is applied on the main loop through done.
	busy string
	done chan func(*model)
	// agentSelection holds the tracked checkboxes while dialogAgents is open.
	agentSelection map[string]bool
}

type keyEvent struct {
	key string
	err error
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

	app := model{service: service, workspace: workspace, focusScopes: true, done: make(chan func(*model))}
	app.showWarnings()
	keys := make(chan keyEvent)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			key, err := readKey(reader)
			keys <- keyEvent{key: key, err: err}
			if err != nil {
				return
			}
		}
	}()
	for {
		app.render()
		select {
		case event := <-keys:
			if event.err == io.EOF {
				return nil
			}
			if event.err != nil {
				return event.err
			}
			if app.busy != "" {
				if event.key == "\x03" {
					return nil
				}
				continue
			}
			if app.handleKey(event.key) {
				return nil
			}
		case apply := <-app.done:
			app.busy = ""
			apply(&app)
		}
	}
}

// runInBackground shows status, runs work off the main loop, and applies the
// closure it returns once finished. work must not touch the model directly.
func (m *model) runInBackground(status string, work func() func(*model)) {
	m.busy = status
	m.setMessage(status)
	done := m.done
	go func() { done <- work() }()
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
		if m.selectedSkill() != nil && len(m.scopes()) > 1 {
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
	case "s":
		if m.selectedScope() != nil {
			m.dialog, m.input = dialogRegistrySearch, ""
		}
	case "a":
		m.agentSelection = map[string]bool{}
		for _, agent := range m.workspace.Agents {
			m.agentSelection[agent.ID] = agent.Tracked
		}
		m.dialog, m.dialogIndex = dialogAgents, 0
	}
	return false
}

func (m *model) handleDialogKey(key string) {
	if key == "esc" {
		m.dialog, m.input, m.registry, m.registryChoice = dialogNone, "", nil, 0
		return
	}
	if m.dialog == dialogProject || m.dialog == dialogInstallURL || m.dialog == dialogRegistrySearch {
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
	if m.dialog == dialogAgents && key == " " {
		if m.dialogIndex < len(m.workspace.Agents) {
			id := m.workspace.Agents[m.dialogIndex].ID
			m.agentSelection[id] = !m.agentSelection[id]
		}
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
	case dialogRegistrySearch:
		query := strings.TrimSpace(m.input)
		if len([]rune(query)) < 2 {
			m.setError("Type at least 2 characters to search skills.sh.")
			return
		}
		service := m.service
		m.runInBackground("Searching skills.sh for \""+query+"\"...", func() func(*model) {
			results, err := service.SearchRegistrySkills(query)
			return func(m *model) {
				if err != nil {
					m.setError(err.Error())
					return
				}
				if len(results) == 0 {
					m.setError("No skills.sh results for \"" + query + "\".")
					return
				}
				m.registry, m.dialog, m.dialogIndex = results, dialogRegistryResults, 0
				m.setMessage(fmt.Sprintf("%d skills.sh results for \"%s\".", len(results), query))
			}
		})
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
		if skill == nil || len(m.scopes()) == 0 {
			return
		}
		target := m.scopes()[m.dialogIndex]
		workspace, err := m.service.CopySkill(skill.Path, target.ID)
		m.applyWorkspace(workspace, err, "Skill copied.")
	case dialogDelete:
		if skill == nil {
			return
		}
		workspace, err := m.service.DeleteSkill(skill.Path)
		m.applyWorkspace(workspace, err, "Skill deleted.")
	case dialogInstallTo:
		if len(m.scopes()) == 0 {
			return
		}
		service, installURL, targetID := m.service, m.installURL, m.scopes()[m.dialogIndex].ID
		m.runInBackground("Installing skill...", func() func(*model) {
			result, err := service.InstallSkillFromURL(installURL, targetID)
			return func(m *model) {
				if err != nil {
					m.setError(err.Error())
					return
				}
				m.workspace, m.dialog, m.installURL = result.Workspace, dialogNone, ""
				m.normalizeSelection()
				m.setMessage("Skill installed via " + result.Method + ".")
			}
		})
	case dialogAgents:
		var tracked []string
		for _, agent := range m.workspace.Agents {
			if m.agentSelection[agent.ID] {
				tracked = append(tracked, agent.ID)
			}
		}
		workspace, err := m.service.SetTrackedAgents(tracked)
		m.applyWorkspace(workspace, err, "Tracked agents updated.")
	case dialogRegistryResults:
		if len(m.registry) == 0 {
			return
		}
		m.registryChoice, m.dialog, m.dialogIndex = m.dialogIndex, dialogRegistryTarget, m.scopeIndex
	case dialogRegistryTarget:
		if len(m.scopes()) == 0 || m.registryChoice >= len(m.registry) {
			return
		}
		service, skill, target := m.service, m.registry[m.registryChoice], m.scopes()[m.dialogIndex]
		m.runInBackground("Installing "+skill.Name+" from skills.sh...", func() func(*model) {
			result, err := service.InstallRegistrySkill(skill.Source, skill.SkillID, target.ID)
			return func(m *model) {
				if err != nil {
					m.setError(err.Error())
					return
				}
				m.workspace, m.dialog, m.registry, m.registryChoice = result.Workspace, dialogNone, nil, 0
				m.normalizeSelection()
				m.setMessage(skill.Name + " from skills.sh installed into " + target.Name + " via " + result.Method + ".")
			}
		})
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
	m.showWarnings()
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
		m.scopeIndex = boundedIndex(m.scopeIndex+offset, len(m.scopes()))
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
	case dialogCopy, dialogInstallTo, dialogRegistryTarget:
		limit = len(m.scopes())
	case dialogRegistryResults:
		limit = len(m.registry)
	case dialogAgents:
		limit = len(m.workspace.Agents)
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
	m.scopeIndex = boundedIndex(m.scopeIndex, len(m.scopes()))
	m.skillIndex = boundedIndex(m.skillIndex, len(m.skillsForSelectedScope()))
}

// scopes are the lanes the TUI shows: agents the user stopped tracking are
// reported by discovery (so conflicts stay accurate) but not drawn.
func (m *model) scopes() []domain.Scope {
	visible := make([]domain.Scope, 0, len(m.workspace.Scopes))
	for _, scope := range m.workspace.Scopes {
		if !scope.Hidden {
			visible = append(visible, scope)
		}
	}
	return visible
}

// showWarnings surfaces non-fatal discovery problems, such as invalid preferences.
func (m *model) showWarnings() {
	if len(m.workspace.Warnings) > 0 {
		m.setError(strings.Join(m.workspace.Warnings, " "))
	}
}

func (m *model) selectedScope() *domain.Scope {
	if len(m.scopes()) == 0 {
		return nil
	}
	return &m.scopes()[m.scopeIndex]
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
