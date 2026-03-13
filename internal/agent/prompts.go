package agent

import (
	"context"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/config"
)

func architectPrompt(cfg *config.Config, opts ...prompt.Option) (*prompt.Prompt, error) {
	template, err := LoadTemplate("architect.md.tpl", cfg, opts...)
	if err != nil {
		return nil, err
	}
	systemPrompt, err := prompt.NewPrompt("architect", template, opts...)
	if err != nil {
		return nil, err
	}
	return systemPrompt, nil
}

func coderPrompt(cfg *config.Config, opts ...prompt.Option) (*prompt.Prompt, error) {
	template, err := LoadTemplate("coder.md.tpl", cfg, opts...)
	if err != nil {
		return nil, err
	}
	systemPrompt, err := prompt.NewPrompt("coder", template, opts...)
	if err != nil {
		return nil, err
	}
	return systemPrompt, nil
}

func taskPrompt(cfg *config.Config, opts ...prompt.Option) (*prompt.Prompt, error) {
	template, err := LoadTemplate("task.md.tpl", cfg, opts...)
	if err != nil {
		return nil, err
	}
	systemPrompt, err := prompt.NewPrompt("task", template, opts...)
	if err != nil {
		return nil, err
	}
	return systemPrompt, nil
}

func InitializePrompt(store *config.ConfigStore) (string, error) {
	template, err := LoadTemplate("initialize.md.tpl", store.Config())
	if err != nil {
		return "", err
	}
	systemPrompt, err := prompt.NewPrompt("initialize", template)
	if err != nil {
		return "", err
	}
	return systemPrompt.Build(context.Background(), "", "", store)
}
