package channelstore

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

// SelectorOptions controls the selector's random source.
type SelectorOptions struct {
	Random *rand.Rand
}

// PriorityWeightedSelector chooses the best route by priority tier, then uses
// weight within that tier. Mapping fields override channel defaults, and key
// fields can override channel baseURL/region.
type PriorityWeightedSelector struct {
	Source                Store
	AllowKeylessSelection bool

	mu  sync.Mutex
	rng *rand.Rand
}

var _ router.ChannelSelector = (*PriorityWeightedSelector)(nil)

type selectionCandidate struct {
	selection router.ChannelSelection
	weight    int
}

func NewPriorityWeightedSelector(source Store, opts SelectorOptions) *PriorityWeightedSelector {
	selector := &PriorityWeightedSelector{Source: source}
	if opts.Random != nil {
		selector.rng = opts.Random
	}
	return selector
}

func (s *PriorityWeightedSelector) SelectChannel(
	ctx context.Context,
	request *router.ChannelRouteRequest,
	resolution *router.ModelResolution,
	mappings []router.ChannelModelMapping,
) (*router.ChannelSelection, error) {
	if s == nil || s.Source == nil {
		return nil, fmt.Errorf("channelstore selector requires a store source")
	}

	providerHint := firstNonEmpty(providerHintFromResolution(resolution), providerHintFromRequest(request))
	region := firstNonEmpty(regionFromResolution(resolution), regionFromRequest(request))
	excludedChannels := excludedChannelsFromRequest(request)
	excludedKeys := excludedKeysFromRequest(request)

	filteredMappings := filterMappings(mappings, providerHint, region, excludedChannels)
	if len(filteredMappings) == 0 {
		return nil, nil
	}

	channelIndex, err := s.loadChannels(ctx, filteredMappings)
	if err != nil {
		return nil, err
	}

	candidates := make([]selectionCandidate, 0, len(filteredMappings))
	for _, mapping := range filteredMappings {
		channelID := strings.TrimSpace(mapping.ChannelID)
		if channelID == "" {
			continue
		}

		channel := channelIndex[channelID]
		keys, err := s.Source.ListKeys(ctx, channelID)
		if err != nil {
			return nil, err
		}

		if len(keys) == 0 && s.AllowKeylessSelection {
			provider := firstNonEmpty(strings.TrimSpace(mapping.Provider), strings.TrimSpace(channel.Provider))
			candidateRegion := firstNonEmpty(strings.TrimSpace(mapping.Region), strings.TrimSpace(channel.Region))
			if !matchesProvider(provider, providerHint) || !matchesRegion(candidateRegion, region) {
				continue
			}

			priority := resolvePriority(mapping.Priority, channel.Priority)
			weight := resolveWeight(mapping.Weight, channel.Weight)
			candidates = append(candidates, selectionCandidate{
				selection: router.ChannelSelection{
					MappingID:   strings.TrimSpace(mapping.MappingID),
					ChannelID:   channelID,
					Provider:    provider,
					RemoteModel: firstNonEmpty(strings.TrimSpace(mapping.RemoteModel), resolvedModelFromResolution(resolution), requestedModelFrom(request)),
					BaseURL:     firstNonEmpty(strings.TrimSpace(mapping.BaseURL), strings.TrimSpace(channel.BaseURL)),
					Region:      candidateRegion,
					Priority:    priority,
					Weight:      normalizedWeight(weight),
					Metadata:    mergeStringMaps(channel.Metadata, mapping.Metadata),
				},
				weight: normalizedWeight(weight),
			})
			continue
		}

		for _, key := range keys {
			candidate, ok := buildCandidate(mapping, channel, key, request, resolution, excludedChannels, excludedKeys, providerHint, region)
			if !ok {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	bestPriority := normalizedPriority(candidates[0].selection.Priority)
	for _, candidate := range candidates[1:] {
		if priority := normalizedPriority(candidate.selection.Priority); priority < bestPriority {
			bestPriority = priority
		}
	}

	tier := make([]selectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if normalizedPriority(candidate.selection.Priority) == bestPriority {
			tier = append(tier, candidate)
		}
	}

	chosen := chooseWeighted(tier, s.drawIndex)
	selection := chosen.selection
	return &selection, nil
}

func (s *PriorityWeightedSelector) loadChannels(ctx context.Context, mappings []router.ChannelModelMapping) (map[string]Channel, error) {
	channels, err := s.Source.GetChannels(ctx, mappingChannelIDs(mappings))
	if err != nil {
		return nil, err
	}
	index := make(map[string]Channel, len(channels))
	for _, channel := range channels {
		channelID := strings.TrimSpace(channel.ID)
		if channelID == "" {
			continue
		}
		index[channelID] = channel
	}
	return index, nil
}

func filterMappings(
	mappings []router.ChannelModelMapping,
	providerHint string,
	region string,
	excludedChannels map[string]struct{},
) []router.ChannelModelMapping {
	out := make([]router.ChannelModelMapping, 0, len(mappings))
	for _, mapping := range mappings {
		channelID := strings.TrimSpace(mapping.ChannelID)
		if _, excluded := excludedChannels[channelID]; excluded {
			continue
		}
		if !matchesProvider(strings.TrimSpace(mapping.Provider), providerHint) {
			continue
		}
		if !matchesRegion(strings.TrimSpace(mapping.Region), region) {
			continue
		}
		out = append(out, mapping)
	}
	return out
}

func mappingChannelIDs(mappings []router.ChannelModelMapping) []string {
	seen := make(map[string]struct{}, len(mappings))
	out := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		channelID := strings.TrimSpace(mapping.ChannelID)
		if channelID == "" {
			continue
		}
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		out = append(out, channelID)
	}
	return out
}

func buildCandidate(
	mapping router.ChannelModelMapping,
	channel Channel,
	key Key,
	request *router.ChannelRouteRequest,
	resolution *router.ModelResolution,
	excludedChannels map[string]struct{},
	excludedKeys map[string]struct{},
	providerHint string,
	region string,
) (selectionCandidate, bool) {
	if key.Disabled {
		return selectionCandidate{}, false
	}

	keyID := strings.TrimSpace(key.ID)
	channelID := firstNonEmpty(strings.TrimSpace(mapping.ChannelID), strings.TrimSpace(key.ChannelID), strings.TrimSpace(channel.ID))
	if _, excluded := excludedChannels[channelID]; excluded {
		return selectionCandidate{}, false
	}
	if _, excluded := excludedKeys[keyID]; excluded {
		return selectionCandidate{}, false
	}

	provider := firstNonEmpty(strings.TrimSpace(mapping.Provider), strings.TrimSpace(channel.Provider))
	if !matchesProvider(provider, providerHint) {
		return selectionCandidate{}, false
	}

	candidateRegion := firstNonEmpty(strings.TrimSpace(key.Region), strings.TrimSpace(mapping.Region), strings.TrimSpace(channel.Region))
	if !matchesRegion(candidateRegion, region) {
		return selectionCandidate{}, false
	}

	priority := resolvePriority(mapping.Priority, channel.Priority, key.Priority)
	weight := resolveWeight(mapping.Weight, channel.Weight, key.Weight)
	metadata := mergeStringMaps(channel.Metadata, mapping.Metadata, key.Metadata)

	return selectionCandidate{
		selection: router.ChannelSelection{
			MappingID:   strings.TrimSpace(mapping.MappingID),
			ChannelID:   channelID,
			KeyID:       keyID,
			Provider:    provider,
			RemoteModel: firstNonEmpty(strings.TrimSpace(mapping.RemoteModel), resolvedModelFromResolution(resolution), requestedModelFrom(request)),
			BaseURL:     firstNonEmpty(strings.TrimSpace(key.BaseURL), strings.TrimSpace(mapping.BaseURL), strings.TrimSpace(channel.BaseURL)),
			Region:      candidateRegion,
			Priority:    priority,
			Weight:      weight,
			Metadata:    metadata,
		},
		weight: weight,
	}, true
}

func chooseWeighted(candidates []selectionCandidate, pick func(totalWeight int) int) selectionCandidate {
	if len(candidates) == 1 {
		return candidates[0]
	}

	total := 0
	for _, candidate := range candidates {
		total += normalizedWeight(candidate.weight)
	}
	if total <= 0 {
		return candidates[0]
	}

	target := pick(total)
	if target < 0 {
		target = 0
	}
	if target >= total {
		target = total - 1
	}

	acc := 0
	for _, candidate := range candidates {
		acc += normalizedWeight(candidate.weight)
		if target < acc {
			return candidate
		}
	}
	return candidates[len(candidates)-1]
}

func resolvePriority(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func resolveWeight(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 1
}

func normalizedWeight(weight int) int {
	if weight <= 0 {
		return 1
	}
	return weight
}

func normalizedPriority(priority int) int {
	if priority > 0 {
		return priority
	}
	return 100
}

func regionRank(candidate string, requested string) int {
	if strings.TrimSpace(requested) == "" {
		return 0
	}
	switch {
	case strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(requested)):
		return 0
	case strings.TrimSpace(candidate) == "":
		return 1
	default:
		return 2
	}
}

func (s *PriorityWeightedSelector) drawIndex(totalWeight int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rng == nil {
		s.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return s.rng.Intn(totalWeight)
}

func providerHintFromRequest(request *router.ChannelRouteRequest) string {
	if request == nil {
		return ""
	}
	return strings.TrimSpace(request.ProviderHint)
}

func providerHintFromResolution(resolution *router.ModelResolution) string {
	if resolution == nil {
		return ""
	}
	return strings.TrimSpace(resolution.ProviderHint)
}

func regionFromRequest(request *router.ChannelRouteRequest) string {
	if request == nil {
		return ""
	}
	return strings.TrimSpace(request.Region)
}

func regionFromResolution(resolution *router.ModelResolution) string {
	if resolution == nil {
		return ""
	}
	return strings.TrimSpace(resolution.Region)
}

func requestedModelFrom(request *router.ChannelRouteRequest) string {
	if request == nil {
		return ""
	}
	return strings.TrimSpace(request.RequestedModel)
}

func resolvedModelFromResolution(resolution *router.ModelResolution) string {
	if resolution == nil {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(resolution.ResolvedModel), strings.TrimSpace(resolution.RequestedModel))
}

func excludedChannelsFromRequest(request *router.ChannelRouteRequest) map[string]struct{} {
	return toSet(request, true)
}

func excludedKeysFromRequest(request *router.ChannelRouteRequest) map[string]struct{} {
	return toSet(request, false)
}

func toSet(request *router.ChannelRouteRequest, channels bool) map[string]struct{} {
	if request == nil {
		return nil
	}
	var values []string
	if channels {
		values = request.ExcludedChannelIDs
	} else {
		values = request.ExcludedKeyIDs
	}
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	return set
}

func matchesProvider(candidate string, hint string) bool {
	if strings.TrimSpace(hint) == "" {
		return true
	}
	if strings.TrimSpace(candidate) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(hint))
}

func matchesRegion(candidate string, requested string) bool {
	if strings.TrimSpace(requested) == "" {
		return true
	}
	if strings.TrimSpace(candidate) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(requested))
}
