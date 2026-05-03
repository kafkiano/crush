package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
)

//go:embed templates/coder_tool.md
var coderToolDescription []byte

type CoderParams struct {
	Prompt string `json:"prompt" description:"The implementation task for the coder to perform. Include all context: what to implement, which files, constraints, and test expectations."`
}

const (
	CoderToolName = "coder"
)

func (c *coordinator) coderTool(ctx context.Context) (fantasy.AgentTool, error) {
	agentCfg, ok := c.cfg.Config().Agents[config.AgentCoder]
	if !ok {
		return nil, errors.New("coder agent not configured")
	}
	coderPrompt, err := coderPrompt(c.cfg.Config(), prompt.WithWorkingDir(c.cfg.WorkingDir()))
	if err != nil {
		return nil, err
	}

	agent, err := c.buildAgent(ctx, coderPrompt, agentCfg, true)
	if err != nil {
		return nil, err
	}
	return fantasy.NewParallelAgentTool(
		CoderToolName,
		tools.FirstLineDescription(coderToolDescription),
		func(ctx context.Context, params CoderParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Prompt == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			sessionID := tools.GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, errors.New("session id missing from context")
			}

			agentMessageID := tools.GetMessageFromContext(ctx)
			if agentMessageID == "" {
				return fantasy.ToolResponse{}, errors.New("agent message id missing from context")
			}

			response, err := c.runSubAgent(ctx, subAgentParams{
				Agent:          agent,
				SessionID:      sessionID,
				AgentMessageID: agentMessageID,
				ToolCallID:     call.ID,
				Prompt:         params.Prompt,
				SessionTitle:   "Coder Delegation",
				SessionSetup: func(sessionID string) {
					c.permissions.AutoApproveSession(sessionID)
				},
			})
			if err != nil {
				return response, err
			}

			// Enrich with git diff stats
			diffStats := c.getGitDiffStats()
			if diffStats != "" {
				enriched := fmt.Sprintf("%s\n\n--- Changes ---\n%s", response.Content, diffStats)
				return fantasy.NewTextResponse(enriched), nil
			}
			return response, nil
		}), nil
}
