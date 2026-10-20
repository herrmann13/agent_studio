package main

import (
	"context"
	"fmt"

	"github.com/herrmann13/agent_studio/internal/application"
	"github.com/herrmann13/agent_studio/internal/domain"
	"github.com/herrmann13/agent_studio/internal/update"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx       context.Context
	discovery *application.DiscoveryService
}

// NewApp creates a new App application struct
func NewApp() *App {
	discovery, err := application.DefaultDiscoveryService()
	if err != nil {
		panic(fmt.Sprintf("create discovery service: %v", err))
	}
	return &App{discovery: discovery}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ApplicationName provides frontend metadata without exposing implementation details.
func (a *App) ApplicationName() string {
	return "Agent Studio"
}

// CheckForUpdates compares the installed build with the latest GitHub Release.
func (a *App) CheckForUpdates() (update.Info, error) {
	return update.Check(context.Background(), version)
}

// DownloadAndInstallUpdate validates and starts installation of the checked release.
func (a *App) DownloadAndInstallUpdate(tagName string) error {
	if err := update.DownloadAndInstall(context.Background(), version, tagName); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
	return nil
}

// DiscoverLocalEnvironment returns a read-only inventory of supported local agents and skills.
func (a *App) DiscoverLocalEnvironment() (domain.DiscoveryResult, error) {
	return a.discovery.Discover()
}

// AddProject starts tracking a project skill directory without modifying the project.
func (a *App) AddProject(path string) (domain.DiscoveryResult, error) {
	return a.discovery.AddProject(path)
}

// InstallSkillFromURL installs a public repository skill into a selected scope.
func (a *App) InstallSkillFromURL(rawURL, targetScopeID string) (domain.SkillInstallResult, error) {
	return a.discovery.InstallSkillFromURL(rawURL, targetScopeID)
}

// RemoveProject stops tracking a project without deleting any project files.
func (a *App) RemoveProject(projectID string) (domain.DiscoveryResult, error) {
	return a.discovery.RemoveProject(projectID)
}

// SelectProjectFolder opens the native directory picker used to choose a project.
func (a *App) SelectProjectFolder() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("application is not ready")
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Select a project folder",
		CanCreateDirectories: false,
		ShowHiddenFiles:      true,
	})
}

// CopySkill copies a complete skill directory into an explicit destination scope.
func (a *App) CopySkill(skillPath, targetScopeID string) (domain.DiscoveryResult, error) {
	return a.discovery.CopySkill(skillPath, targetScopeID)
}

// SearchRegistrySkills searches the public skills.sh directory.
func (a *App) SearchRegistrySkills(query string) ([]domain.RegistrySkill, error) {
	return a.discovery.SearchRegistrySkills(query)
}

// InstallRegistrySkill installs a skills.sh search result into a selected scope.
func (a *App) InstallRegistrySkill(source, skillID, targetScopeID string) (domain.SkillInstallResult, error) {
	return a.discovery.InstallRegistrySkill(source, skillID, targetScopeID)
}

// SyncGlobalSkills copies Global skills to installed agents that cannot read them natively.
func (a *App) SyncGlobalSkills() (domain.DiscoveryResult, error) {
	return a.discovery.SyncGlobalSkills()
}

// DeleteSkill permanently removes a skill after the frontend obtains explicit confirmation.
func (a *App) DeleteSkill(skillPath string) (domain.DiscoveryResult, error) {
	return a.discovery.DeleteSkill(skillPath)
}

// SetSkillInvocationMode configures how an agent may use one skill copy.
func (a *App) SetSkillInvocationMode(skillPath, mode string) (domain.DiscoveryResult, error) {
	return a.discovery.SetSkillInvocationMode(skillPath, mode)
}
