package agent

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/config"
)

//go:embed templates/coder.md.tpl
var coderPromptTmpl []byte

//go:embed templates/task.md.tpl
var taskPromptTmpl []byte

//go:embed templates/architect.md.tpl
var architectPromptTmpl []byte

//go:embed templates/initialize.md.tpl
var initializePromptTmpl []byte

// LoadTemplate loads a template from disk or falls back to embedded template.
// It searches for the template file in the configured template paths,
// and if not found, returns the embedded template.
func LoadTemplate(name string, cfg *config.Config, opts ...prompt.Option) (string, error) {
	// First, try to load from configured template paths
	if cfg != nil && cfg.Options != nil {
		for _, dir := range cfg.Options.TemplatePaths {
			// Expand ~ to home directory
			expandedDir := dir
			if strings.HasPrefix(dir, "~") {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					continue
				}
				expandedDir = filepath.Join(homeDir, dir[1:])
			}

			// Construct template file path
			templatePath := filepath.Join(expandedDir, name)

			// Try to read the template file
			if data, err := os.ReadFile(templatePath); err == nil {
				return string(data), nil
			}
		}
	}

	// Fall back to embedded template
	switch name {
	case "coder.md.tpl":
		return string(coderPromptTmpl), nil
	case "task.md.tpl":
		return string(taskPromptTmpl), nil
	case "architect.md.tpl":
		return string(architectPromptTmpl), nil
	case "initialize.md.tpl":
		return string(initializePromptTmpl), nil
	default:
		return "", fmt.Errorf("unknown template: %s", name)
	}
}
