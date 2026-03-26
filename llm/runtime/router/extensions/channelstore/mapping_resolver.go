package channelstore

import (
	"context"
	"sort"
	"strings"

	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/types"
)

// StoreModelMappingResolver adapts a generic mapping source to the framework's
// ModelMappingResolver interface.
type StoreModelMappingResolver struct {
	Source ModelMappingSource
}

// ResolveMappings prefers exact public-model matches and falls back to provider
// matches when a provider hint is available.
func (r StoreModelMappingResolver) ResolveMappings(ctx context.Context, request *llmrouter.ChannelRouteRequest, resolution *llmrouter.ModelResolution) ([]llmrouter.ChannelModelMapping, error) {
	if r.Source == nil {
		return nil, types.NewServiceUnavailableError("channelstore mapping resolver requires a mapping source")
	}

	requestedModel := firstNonEmpty(modelResolutionValue(resolution), requestValue(request))
	if requestedModel == "" {
		return nil, types.NewInvalidRequestError("channelstore mapping resolver requires a requested model")
	}

	modelMappings, err := r.Source.FindMappingsByModel(ctx, requestedModel)
	if err != nil {
		return nil, err
	}
	if len(modelMappings) != 0 {
		return toRouterMappings(modelMappings, normalizedRegion(request, resolution)), nil
	}

	providerHint := firstNonEmpty(providerResolutionValue(resolution), providerHintValue(request))
	if providerHint == "" {
		return nil, nil
	}

	providerMappings, err := r.Source.FindMappingsByProvider(ctx, providerHint)
	if err != nil {
		return nil, err
	}
	return toRouterMappings(providerMappings, normalizedRegion(request, resolution)), nil
}

func toRouterMappings(records []ModelMapping, region string) []llmrouter.ChannelModelMapping {
	if len(records) == 0 {
		return nil
	}

	sorted := make([]ModelMapping, 0, len(records))
	for _, record := range records {
		if record.Disabled {
			continue
		}
		sorted = append(sorted, cloneModelMapping(record))
	}

	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i]
		right := sorted[j]
		leftRegionRank := regionRank(left.Region, region)
		rightRegionRank := regionRank(right.Region, region)
		if leftRegionRank != rightRegionRank {
			return leftRegionRank < rightRegionRank
		}
		leftPriority := normalizedPriority(left.Priority)
		rightPriority := normalizedPriority(right.Priority)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftWeight := normalizedWeight(left.Weight)
		rightWeight := normalizedWeight(right.Weight)
		if leftWeight != rightWeight {
			return leftWeight > rightWeight
		}
		return firstNonEmpty(left.ID, left.ChannelID) < firstNonEmpty(right.ID, right.ChannelID)
	})

	mappings := make([]llmrouter.ChannelModelMapping, 0, len(sorted))
	for _, record := range sorted {
		mappings = append(mappings, llmrouter.ChannelModelMapping{
			MappingID:   strings.TrimSpace(record.ID),
			ChannelID:   strings.TrimSpace(record.ChannelID),
			Provider:    strings.TrimSpace(record.Provider),
			PublicModel: strings.TrimSpace(record.PublicModel),
			RemoteModel: strings.TrimSpace(record.RemoteModel),
			BaseURL:     strings.TrimSpace(record.BaseURL),
			Region:      strings.TrimSpace(record.Region),
			Priority:    record.Priority,
			Weight:      record.Weight,
			Metadata:    cloneStringMap(record.Metadata),
		})
	}
	return mappings
}

func requestValue(request *llmrouter.ChannelRouteRequest) string {
	if request == nil {
		return ""
	}
	return request.RequestedModel
}

func providerHintValue(request *llmrouter.ChannelRouteRequest) string {
	if request == nil {
		return ""
	}
	return request.ProviderHint
}

func modelResolutionValue(resolution *llmrouter.ModelResolution) string {
	if resolution == nil {
		return ""
	}
	return firstNonEmpty(resolution.ResolvedModel, resolution.RequestedModel)
}

func providerResolutionValue(resolution *llmrouter.ModelResolution) string {
	if resolution == nil {
		return ""
	}
	return resolution.ProviderHint
}

func normalizedRegion(request *llmrouter.ChannelRouteRequest, resolution *llmrouter.ModelResolution) string {
	if resolution != nil && strings.TrimSpace(resolution.Region) != "" {
		return strings.TrimSpace(resolution.Region)
	}
	if request != nil {
		return strings.TrimSpace(request.Region)
	}
	return ""
}
