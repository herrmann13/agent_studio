package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/herrmann13/agent_studio/internal/adapters"
	"github.com/herrmann13/agent_studio/internal/domain"
)

// projectRootFromScopeRoot derives the project directory from a project scope's
// Root (which is <project>/.agents/skills).
func projectRootFromScopeRoot(scopeRoot string) string {
	return filepath.Dir(filepath.Dir(scopeRoot))
}

// globalCopyMarker marks a user-level copy that Agent Studio made from the Global
// scope. Only marked copies are ever replaced or removed, and they are hidden from
// that agent's own lane so the skill is listed once, under Global.
const globalCopyMarker = ".agent-studio-global"

func (s *DiscoveryService) globalSkillRoot() string {
	return filepath.Join(s.home, ".agents", "skills")
}

// globalCopyRoots lists the user-level skill directories of agents that cannot
// read `~/.agents/skills` themselves (today only Claude Code). Copies are only
// made for installed agents; cleanup also covers agents uninstalled since.
func (s *DiscoveryService) globalCopyRoots(installedOnly bool) []string {
	needsCopy := providersWithIndependentSkillCopy()
	var roots []string
	for _, adapter := range s.adapters {
		if !needsCopy[adapter.Provider()] || (installedOnly && len(adapter.Detect(s.env)) == 0) {
			continue
		}
		roots = append(roots, adapter.UserSkillRoot(s.home))
	}
	return roots
}

// globalTarget classifies what already sits at an agent's copy of a global skill.
type globalTarget int

const (
	globalTargetMissing globalTarget = iota
	// globalTargetManaged is a marked copy made by Agent Studio.
	globalTargetManaged
	// globalTargetLinked is a symlink to the global skill, as `npx skills` creates;
	// the agent already reads the canonical copy through it.
	globalTargetLinked
	// globalTargetForeign is the agent's own skill, which is never touched.
	globalTargetForeign
)

func globalTargetState(destination, globalDir string) (globalTarget, error) {
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return globalTargetMissing, nil
	}
	if err != nil {
		return 0, fmt.Errorf("inspect %s: %w", destination, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if symlinkPointsTo(destination, globalDir) {
			return globalTargetLinked, nil
		}
		return globalTargetForeign, nil
	}
	if isGlobalCopy(destination) {
		return globalTargetManaged, nil
	}
	return globalTargetForeign, nil
}

// symlinkTarget returns where a link points, made absolute but not resolved, so
// it also works for a link whose target was just removed (dangling).
func symlinkTarget(link string) (string, bool) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	return filepath.Clean(target), true
}

func symlinkPointsTo(link, directory string) bool {
	if resolvedLink, err := filepath.EvalSymlinks(link); err == nil {
		if resolvedDirectory, err := filepath.EvalSymlinks(directory); err == nil {
			return resolvedLink == resolvedDirectory
		}
	}
	target, ok := symlinkTarget(link)
	return ok && target == filepath.Clean(directory)
}

// checkGlobalCopyTargets refuses before anything is written when an installed
// agent already has its own, unmanaged skill with the same folder name.
func (s *DiscoveryService) checkGlobalCopyTargets(scope domain.Scope, skillDirName string) error {
	if scope.Kind != "global" {
		return nil
	}
	_, err := classifyGlobalTargets(s.globalCopyRoots(true), filepath.Join(s.globalSkillRoot(), skillDirName))
	return err
}

// classifyGlobalTargets inspects each root once and refuses if any of them holds
// the agent's own skill under the same name.
func classifyGlobalTargets(roots []string, globalDir string) ([]globalTarget, error) {
	states := make([]globalTarget, len(roots))
	for index, root := range roots {
		state, err := globalTargetState(filepath.Join(root, filepath.Base(globalDir)), globalDir)
		if err != nil {
			return nil, err
		}
		if state == globalTargetForeign {
			return nil, fmt.Errorf("%s already contains its own %q skill; remove or rename it before making it global", root, filepath.Base(globalDir))
		}
		states[index] = state
	}
	return states, nil
}

func propagateToGlobalRoots(roots []string, sourceDir string) error {
	states, err := classifyGlobalTargets(roots, sourceDir)
	if err != nil {
		return err
	}
	for index, root := range roots {
		if states[index] == globalTargetLinked {
			continue
		}
		destination := filepath.Join(root, filepath.Base(sourceDir))
		if err := copyDirectoryForce(sourceDir, destination); err != nil {
			return fmt.Errorf("propagate skill to %s: %w", root, err)
		}
		if err := os.WriteFile(filepath.Join(destination, globalCopyMarker), []byte(globalCopyMarkerPrefix+sourceDir+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// removeGlobalCopies removes a deleted global skill's marked copies and the
// symlinks that pointed to it, so it never lingers in any agent. globalDir may be
// nested below `~/.agents/skills`; copies are named after its last element.
func (s *DiscoveryService) removeGlobalCopies(globalDir string) error {
	for _, root := range s.globalCopyRoots(false) {
		destination := filepath.Join(root, filepath.Base(globalDir))
		state, err := globalTargetState(destination, globalDir)
		if err != nil {
			return err
		}
		switch {
		case state == globalTargetManaged && s.globalCopySource(destination) == filepath.Clean(globalDir):
			err = os.RemoveAll(destination)
		case state == globalTargetLinked:
			err = os.Remove(destination)
		}
		if err != nil {
			return fmt.Errorf("remove global copy %s: %w", destination, err)
		}
	}
	return nil
}

// removeOrphanedGlobalCopies deletes marked copies, and links into
// `~/.agents/skills`, whose global skill was removed outside Agent Studio. Marked
// copies are hidden from agent lanes, so without this the agent would keep
// loading a skill nobody can see.
func (s *DiscoveryService) removeOrphanedGlobalCopies() error {
	globalRoot := filepath.Clean(s.globalSkillRoot())
	for _, root := range s.globalCopyRoots(false) {
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", root, err)
		}
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			var source string
			if entry.Type()&os.ModeSymlink != 0 {
				target, ok := symlinkTarget(path)
				if !ok || !strings.HasPrefix(target, globalRoot+string(filepath.Separator)) {
					continue
				}
				source = target
			} else if entry.IsDir() && isGlobalCopy(path) {
				source = s.globalCopySource(path)
			} else {
				continue
			}
			if _, err := os.Stat(filepath.Join(source, "SKILL.md")); !os.IsNotExist(err) {
				continue
			}
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove orphaned global copy %s: %w", path, err)
			}
		}
	}
	return nil
}

const globalCopyMarkerPrefix = "Managed by Agent Studio: copy of "

// globalCopySource reads the global skill a marked copy was made from. Markers
// written before the source was recorded fall back to `~/.agents/skills/<name>`.
func (s *DiscoveryService) globalCopySource(copyDir string) string {
	content, err := os.ReadFile(filepath.Join(copyDir, globalCopyMarker))
	if err == nil {
		if source := strings.TrimSpace(strings.TrimPrefix(string(content), globalCopyMarkerPrefix)); source != "" && filepath.IsAbs(source) {
			return filepath.Clean(source)
		}
	}
	return filepath.Join(s.globalSkillRoot(), filepath.Base(copyDir))
}

func isGlobalCopy(skillDir string) bool {
	_, err := os.Stat(filepath.Join(skillDir, globalCopyMarker))
	return err == nil
}

// propagateToProjectAgents copies a skill directory into every agent's canonical
// project-level skill location so all supported agents can use it inside the project.
// The source (`.agents/skills/<skill>`) remains the canonical copy and wins: existing
// propagated copies are replaced unconditionally.
func (s *DiscoveryService) propagateToProjectAgents(sourceDir, projectPath string) error {
	return s.propagateToAgentRoots(sourceDir, func(adapter adapters.Adapter) string {
		return adapter.ProjectSkillRoot(projectPath)
	})
}

// providersWithIndependentSkillCopy lists the agents that actually need a physical,
// separate copy of a Project skill, rather than already seeing it at its
// canonical `.agents/skills` path. Confirmed empirically against the real CLIs, not
// just their docs (which turned out to be incomplete for Codex and OpenCode):
//   - Claude Code only ever reads `~/.claude/skills` and `.claude/skills` — it has no
//     knowledge of `.agents/skills`, so it needs a real copy to see the skill at all.
//   - Codex reads `.agents/skills` (repo scope, walking to the repository root) and
//     `$HOME/.agents/skills` (user scope) directly — the same canonical directory
//     Agent Studio already uses for Project scope. It ALSO reads its own
//     `.codex/skills`/`$CODEX_HOME/skills` (undocumented, found by running
//     `codex debug prompt-input`), but does not deduplicate: a copy placed there
//     under the same name is listed twice.
//   - OpenCode reads `.agents/skills` and `.claude/skills` natively too, alongside
//     its own directory. Unlike Codex it does deduplicate by
//     name, but (confirmed with `opencode debug skill`) it prefers its own copy over
//     the external one — so a stale propagated copy would silently shadow edits made
//     to the canonical skill instead of erroring or disappearing.
//
// Either way, copying into Codex's or OpenCode's own directory on top of the
// canonical one is actively worse than not copying at all, so only Claude gets one.
func providersWithIndependentSkillCopy() map[domain.Provider]bool {
	return map[domain.Provider]bool{domain.ProviderClaude: true}
}

func (s *DiscoveryService) propagateToAgentRoots(sourceDir string, rootFor func(adapters.Adapter) string) error {
	skillDirName := filepath.Base(sourceDir)
	needsCopy := providersWithIndependentSkillCopy()
	for _, adapter := range s.adapters {
		if !needsCopy[adapter.Provider()] {
			continue
		}
		root := rootFor(adapter)
		if root == "" {
			continue
		}
		destination := filepath.Join(root, skillDirName)
		if err := copyDirectoryForce(sourceDir, destination); err != nil {
			return fmt.Errorf("propagate skill to %s: %w", root, err)
		}
	}
	return nil
}

// removePropagatedCopies removes a skill's propagated copies from each agent's
// project-level skill location without touching the canonical `.agents/skills` copy.
func (s *DiscoveryService) removePropagatedCopies(projectPath, skillDirName string) error {
	return s.removePropagatedCopiesFromRoots(skillDirName, func(adapter adapters.Adapter) string {
		return adapter.ProjectSkillRoot(projectPath)
	})
}

func (s *DiscoveryService) removePropagatedCopiesFromRoots(skillDirName string, rootFor func(adapters.Adapter) string) error {
	needsCopy := providersWithIndependentSkillCopy()
	for _, adapter := range s.adapters {
		if !needsCopy[adapter.Provider()] {
			continue
		}
		root := rootFor(adapter)
		if root == "" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, skillDirName)); err != nil {
			return fmt.Errorf("remove propagated skill from %s: %w", root, err)
		}
	}
	return nil
}

// copyDirectoryForce copies source into destination, replacing destination first so
// the operation is idempotent even when the destination already exists.
func copyDirectoryForce(source, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	return copyDirectory(source, destination)
}
