package runtimepolicy

import (
	"context"
	"fmt"
	"sync"
	"time"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/types"
)

// CooldownDecision describes which route scopes should enter cooldown after one usage record.
type CooldownDecision struct {
	KeyTTL     time.Duration
	ChannelTTL time.Duration
}

// CooldownDecider decides whether a usage record should trigger cooldown.
type CooldownDecider interface {
	DecideCooldown(ctx context.Context, usage *router.ChannelUsageRecord) (CooldownDecision, error)
}

// FailureCooldownDecider applies static key/channel cooldowns to failed attempts.
type FailureCooldownDecider struct {
	KeyTTL     time.Duration
	ChannelTTL time.Duration
	Match      func(*router.ChannelUsageRecord) bool
}

// DecideCooldown implements CooldownDecider.
func (d FailureCooldownDecider) DecideCooldown(_ context.Context, usage *router.ChannelUsageRecord) (CooldownDecision, error) {
	if usage == nil || usage.Success {
		return CooldownDecision{}, nil
	}
	if d.Match != nil && !d.Match(usage) {
		return CooldownDecision{}, nil
	}
	return CooldownDecision{
		KeyTTL:     d.KeyTTL,
		ChannelTTL: d.ChannelTTL,
	}, nil
}

// CooldownSnapshot exposes a cloned view of the in-memory cooldown state.
type CooldownSnapshot struct {
	KeyCooldowns     map[string]time.Time
	ChannelCooldowns map[string]time.Time
}

// InMemoryCooldownController is a generic reference cooldown controller.
type InMemoryCooldownController struct {
	Decider CooldownDecider
	Now     func() time.Time

	mu               sync.Mutex
	keyCooldowns     map[string]time.Time
	channelCooldowns map[string]time.Time
}

var _ router.CooldownController = (*InMemoryCooldownController)(nil)

// Allow implements router.CooldownController.
func (c *InMemoryCooldownController) Allow(_ context.Context, _ *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	if selection == nil {
		return nil
	}

	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)

	if keyID := selection.KeyID; keyID != "" {
		if until, ok := c.keyCooldowns[keyID]; ok && until.After(now) {
			return types.NewRateLimitError(fmt.Sprintf("channel route key %s is cooling down", keyID))
		}
	}
	if channelID := selection.ChannelID; channelID != "" {
		if until, ok := c.channelCooldowns[channelID]; ok && until.After(now) {
			return types.NewRateLimitError(fmt.Sprintf("channel route channel %s is cooling down", channelID))
		}
	}
	return nil
}

// RecordResult implements router.CooldownController.
func (c *InMemoryCooldownController) RecordResult(ctx context.Context, usage *router.ChannelUsageRecord) error {
	if c.Decider == nil || usage == nil {
		return nil
	}

	decision, err := c.Decider.DecideCooldown(ctx, cloneUsageRecord(usage))
	if err != nil {
		return err
	}
	if decision.KeyTTL <= 0 && decision.ChannelTTL <= 0 {
		return nil
	}

	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()

	if usage.KeyID != "" && decision.KeyTTL > 0 {
		c.ensureMapsLocked()
		c.keyCooldowns[usage.KeyID] = now.Add(decision.KeyTTL)
	}
	if usage.ChannelID != "" && decision.ChannelTTL > 0 {
		c.ensureMapsLocked()
		c.channelCooldowns[usage.ChannelID] = now.Add(decision.ChannelTTL)
	}
	return nil
}

// Snapshot returns cloned cooldown maps with expired entries removed.
func (c *InMemoryCooldownController) Snapshot() CooldownSnapshot {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)

	return CooldownSnapshot{
		KeyCooldowns:     cloneTimeMap(c.keyCooldowns),
		ChannelCooldowns: cloneTimeMap(c.channelCooldowns),
	}
}

func (c *InMemoryCooldownController) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *InMemoryCooldownController) ensureMapsLocked() {
	if c.keyCooldowns == nil {
		c.keyCooldowns = map[string]time.Time{}
	}
	if c.channelCooldowns == nil {
		c.channelCooldowns = map[string]time.Time{}
	}
}

func (c *InMemoryCooldownController) pruneLocked(now time.Time) {
	for keyID, until := range c.keyCooldowns {
		if !until.After(now) {
			delete(c.keyCooldowns, keyID)
		}
	}
	for channelID, until := range c.channelCooldowns {
		if !until.After(now) {
			delete(c.channelCooldowns, channelID)
		}
	}
}

func cloneTimeMap(values map[string]time.Time) map[string]time.Time {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]time.Time, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
