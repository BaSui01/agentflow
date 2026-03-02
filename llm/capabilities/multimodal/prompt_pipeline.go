package multimodal

import (
	"context"
	"strings"
)

// PromptContext carries modality-specific prompt build options.
type PromptContext struct {
	Modality       string
	BasePrompt     string
	Advanced       bool
	StyleTokens    []string
	QualityTokens  []string
	NegativePrompt string
	Camera         string
	Mood           string
}

// PromptResult is the normalized prompt output.
type PromptResult struct {
	Prompt         string
	NegativePrompt string
}

// PromptPipeline allows framework users to inject custom prompt composition logic.
type PromptPipeline interface {
	Build(ctx context.Context, in PromptContext) (PromptResult, error)
}

// DefaultPromptPipeline provides a generic, domain-agnostic prompt composition strategy.
type DefaultPromptPipeline struct{}

func (p *DefaultPromptPipeline) Build(ctx context.Context, in PromptContext) (PromptResult, error) {
	_ = ctx
	if !in.Advanced {
		return PromptResult{
			Prompt:         strings.TrimSpace(in.BasePrompt),
			NegativePrompt: strings.TrimSpace(in.NegativePrompt),
		}, nil
	}

	pieces := []string{}
	switch in.Modality {
	case "image":
		pieces = append(pieces, strings.Join(in.StyleTokens, ", "))
		pieces = append(pieces, strings.TrimSpace(in.BasePrompt))
		pieces = append(pieces, strings.Join(in.QualityTokens, ", "))
	case "video":
		if strings.TrimSpace(in.Camera) != "" {
			pieces = append(pieces, "Camera: "+strings.TrimSpace(in.Camera))
		}
		if strings.TrimSpace(in.Mood) != "" {
			pieces = append(pieces, "Mood: "+strings.TrimSpace(in.Mood))
		}
		if len(in.StyleTokens) > 0 {
			pieces = append(pieces, "Style: "+strings.Join(in.StyleTokens, ", "))
		}
		pieces = append(pieces, strings.TrimSpace(in.BasePrompt))
	default:
		pieces = append(pieces, strings.TrimSpace(in.BasePrompt))
	}

	return PromptResult{
		Prompt:         strings.Join(filterEmptyStrings(pieces), ". "),
		NegativePrompt: strings.TrimSpace(in.NegativePrompt),
	}, nil
}

func filterEmptyStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

