package main

import (
	"context"
	"fmt"

	"agent-studio/internal/application"
	"agent-studio/internal/domain"
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

// DiscoverLocalEnvironment returns a read-only inventory of supported local agents and skills.
func (a *App) DiscoverLocalEnvironment() (domain.DiscoveryResult, error) {
	return a.discovery.Discover()
}
