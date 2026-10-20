package tui

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/herrmann13/agent_studio/internal/domain"
	"golang.org/x/sys/unix"
)

const (
	DEFAULT_TERMINAL_WIDTH  = 100
	DEFAULT_TERMINAL_HEIGHT = 30
	MAX_SCOPE_COLUMN_WIDTH  = 34
	COLUMN_GAP              = "  "
	ELLIPSIS                = "…"
	// REGISTRY_VISIBLE_RESULTS keeps the skills.sh list from pushing the panels off screen.
	REGISTRY_VISIBLE_RESULTS = 8
)

// compactCount renders install counts as 1.4M / 61.4K / 950.
func compactCount(value int) string {
	switch {
	case value >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.1fK", float64(value)/1_000)
	default:
		return fmt.Sprint(value)
	}
}

// Estados presentes em quase toda skill só poluem a lista, então ficam apenas nos detalhes.
var HIDDEN_LIST_STATES = map[string]bool{"available": true, "associated": true}

type styledLine struct {
	text  string
	style string
}

func terminalSize() (int, int) {
	size, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || size.Col == 0 || size.Row == 0 {
		return DEFAULT_TERMINAL_WIDTH, DEFAULT_TERMINAL_HEIGHT
	}
	return int(size.Col), int(size.Row)
}

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= width {
		return text
	}
	runes := []rune(text)
	return string(runes[:width-1]) + ELLIPSIS
}

func padRight(text string, width int) string {
	text = truncate(text, width)
	return text + strings.Repeat(" ", width-utf8.RuneCountInString(text))
}

func (line styledLine) render(width int) string {
	text := padRight(line.text, width)
	if line.style == "" {
		return text
	}
	return line.style + text + resetSequence
}

func listWindow(selected, total, height int) (int, int) {
	if total <= height {
		return 0, total
	}
	start := min(max(selected-height/2, 0), total-height)
	return start, start + height
}

func selectableLine(text string, isSelected, isFocused bool) styledLine {
	if !isSelected {
		return styledLine{text: "  " + text}
	}
	if isFocused {
		return styledLine{text: "> " + text, style: selectedColor + boldSequence}
	}
	return styledLine{text: "> " + text, style: boldSequence}
}

func (m *model) scopeLines(height int) []styledLine {
	lines := []styledLine{{text: "SCOPES", style: mutedColor}}
	if len(m.scopes()) == 0 {
		return append(lines, styledLine{text: "  No scopes found.", style: mutedColor})
	}
	start, end := listWindow(m.scopeIndex, len(m.scopes()), height-1)
	for index := start; index < end; index++ {
		lines = append(lines, selectableLine(m.scopes()[index].Name, index == m.scopeIndex, m.focusScopes))
	}
	return lines
}

func skillListLabel(skill domain.Skill, nameWidth int) string {
	mode := skill.InvocationMode
	if mode == "" {
		mode = defaultInvocationMode
	}
	flags := make([]string, 0, len(skill.States))
	for _, state := range skill.States {
		if !HIDDEN_LIST_STATES[state] {
			flags = append(flags, state)
		}
	}
	label := padRight(skill.Name, nameWidth) + COLUMN_GAP + padRight(mode, 9)
	if len(flags) > 0 {
		label += COLUMN_GAP + strings.Join(flags, ", ")
	}
	return label
}

func (m *model) skillLines(height, width int) []styledLine {
	title := "SKILLS"
	if scope := m.selectedScope(); scope != nil {
		title += " · " + scope.Name
	}
	skills := m.skillsForSelectedScope()
	lines := []styledLine{{text: fmt.Sprintf("%s (%d)", title, len(skills)), style: mutedColor}}
	if len(skills) == 0 {
		return append(lines, styledLine{text: "  No skills in this scope.", style: mutedColor})
	}
	nameWidth := min(28, max(width/3, 12))
	start, end := listWindow(m.skillIndex, len(skills), height-1)
	for index := start; index < end; index++ {
		lines = append(lines, selectableLine(skillListLabel(skills[index], nameWidth), index == m.skillIndex, !m.focusScopes))
	}
	return lines
}

func (m *model) detailLines() []styledLine {
	skill := m.selectedSkill()
	if skill == nil {
		return []styledLine{{text: "Select a skill to see its details.", style: mutedColor}}
	}
	return []styledLine{
		{text: skill.Name, style: boldSequence},
		{text: strings.Join(strings.Fields(skill.Description), " ")},
		{text: "States: " + strings.Join(skill.States, ", "), style: mutedColor},
		{text: skill.Path, style: mutedColor},
	}
}

func (m *model) dialogLines() []styledLine {
	lines := []styledLine{{text: dialogTitle(m.dialog), style: boldSequence}}
	switch m.dialog {
	case dialogMode:
		for index, mode := range invocationModes() {
			lines = append(lines, selectableLine(mode, index == m.dialogIndex, true))
		}
	case dialogAgents:
		for index, agent := range m.workspace.Agents {
			box := "[ ]"
			if m.agentSelection[agent.ID] {
				box = "[x]"
			}
			label := box + " " + agent.Name
			if agent.Version != "" {
				label += " · " + agent.Version
			}
			lines = append(lines, selectableLine(label, index == m.dialogIndex, true))
		}
		if len(m.workspace.Agents) == 0 {
			lines = append(lines, styledLine{text: "  No supported agent is installed.", style: mutedColor})
		}
	case dialogRegistryResults:
		start, end := listWindow(m.dialogIndex, len(m.registry), REGISTRY_VISIBLE_RESULTS)
		for index := start; index < end; index++ {
			skill := m.registry[index]
			label := fmt.Sprintf("%s · %s · %s installs", skill.Name, skill.Source, compactCount(skill.Installs))
			lines = append(lines, selectableLine(label, index == m.dialogIndex, true))
		}
	case dialogCopy, dialogInstallTo, dialogRegistryTarget:
		for index, scope := range m.scopes() {
			lines = append(lines, selectableLine(scope.Name, index == m.dialogIndex, true))
		}
	case dialogDelete:
		if skill := m.selectedSkill(); skill != nil {
			lines = append(lines, styledLine{text: "Delete \"" + skill.Name + "\" permanently? [y/N]", style: errorColor})
		}
	case dialogProject, dialogInstallURL, dialogRegistrySearch:
		lines = append(lines, styledLine{text: "> " + m.input + "_", style: selectedColor})
	}
	return lines
}

func (m *model) footerLine() styledLine {
	if m.busy != "" {
		return styledLine{text: "working · input paused · ctrl+c quit", style: mutedColor}
	}
	if m.dialog == dialogNone {
		return styledLine{text: "tab panel · ↑↓ move · m mode · c copy · d delete · i install · s search skills.sh · a agents · p/P add/remove project · r refresh · q quit", style: mutedColor}
	}
	if m.dialog == dialogDelete {
		return styledLine{text: "y confirm · n/esc cancel", style: mutedColor}
	}
	if m.dialog == dialogAgents {
		return styledLine{text: "space toggle · enter save · esc cancel", style: mutedColor}
	}
	return styledLine{text: "enter confirm · esc cancel", style: mutedColor}
}

func (m *model) messageLine() styledLine {
	if m.messageIsError {
		return styledLine{text: m.message, style: errorColor}
	}
	return styledLine{text: m.message, style: successColor}
}

func (m *model) bottomLines() []styledLine {
	if m.dialog != dialogNone {
		return m.dialogLines()
	}
	return m.detailLines()
}

func (m *model) render() {
	width, height := terminalSize()
	bottom := m.bottomLines()
	bodyHeight := max(height-len(bottom)-6, 3)
	scopeWidth := min(MAX_SCOPE_COLUMN_WIDTH, width/3)
	skillWidth := width - scopeWidth - len(COLUMN_GAP)
	scopes, skills := m.scopeLines(bodyHeight), m.skillLines(bodyHeight, skillWidth)
	separator := styledLine{text: strings.Repeat("─", width), style: mutedColor}

	lines := []string{
		styledLine{text: "Agent Studio · terminal", style: boldSequence}.render(width),
		separator.render(width),
	}
	for row := 0; row < bodyHeight; row++ {
		left, right := styledLine{}, styledLine{}
		if row < len(scopes) {
			left = scopes[row]
		}
		if row < len(skills) {
			right = skills[row]
		}
		lines = append(lines, left.render(scopeWidth)+COLUMN_GAP+right.render(skillWidth))
	}
	lines = append(lines, separator.render(width))
	for _, line := range bottom {
		lines = append(lines, line.render(width))
	}
	lines = append(lines, m.messageLine().render(width), m.footerLine().render(width))

	// Um único write com o frame completo evita o efeito de cintilação a cada tecla.
	os.Stdout.WriteString(clearScreenSequence + strings.Join(lines[:min(len(lines), height)], "\r\n"))
}

func dialogTitle(value dialog) string {
	switch value {
	case dialogMode:
		return "Set invocation mode"
	case dialogCopy:
		return "Copy skill to"
	case dialogDelete:
		return "Delete skill"
	case dialogProject:
		return "Add project path"
	case dialogInstallURL:
		return "Install skill URL or npx command"
	case dialogInstallTo:
		return "Install into scope"
	case dialogRegistrySearch:
		return "Search skills.sh"
	case dialogRegistryResults:
		return "skills.sh results"
	case dialogRegistryTarget:
		return "Install skills.sh result into scope"
	case dialogAgents:
		return "Tracked agents (hidden agents keep their files; Global still reaches them)"
	default:
		return ""
	}
}
