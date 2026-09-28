package main

import (
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// Every battle entry, including a restored battle, reloads the host's feature
// settings. The retail save has no community configuration (design §3.2).
func communitySources(opts Options, cs *contentSet) session.CommunitySources {
	saved := loadedSettings()
	player, commandLine := settings.UnitLimitSources(saved.GameplayFeatures, opts.GameplayOverrides, opts.UnitLimit, saved.UnitLimit)
	sources := session.CommunitySources{Player: player, CommandLine: commandLine}
	if cs != nil {
		sources.Content = cs.gameplayFeatures
	}
	return sources
}

func sessionBuilderOptions(saved settings.BuilderOptions) *orders.BuilderOptions {
	saved.Normalize()
	options := orders.DefaultBuilderOptions()
	for i := range options.Guard {
		options.Guard[i] = orders.GuardHomeOption(saved.Guard[i])
		options.Patrol[i] = orders.PatrolWorkOption(saved.Patrol[i])
	}
	return &options
}
