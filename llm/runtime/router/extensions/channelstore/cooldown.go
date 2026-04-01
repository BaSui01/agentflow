package channelstore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

// CooldownConfig controls the cascade cooldown behavior.
type CooldownConfig struct {
	// KeyCooldownDuration is how long a failed key stays in cooldown.
	// Default: 30s
	KeyCooldownDuration time.Duration

	// ChannelCooldownDuration is how long a channel stays in cooldown when all
	// its keys have failed (cascade cooldown).
	// Default: 5m
	ChannelCooldownDuration time.Duration

	// FailureThreshold is the number of consecutive failures before a key
	// enters cooldown. Default: 1 (immediate cooldown on first failure).
	FailureThreshold int

	// ChannelFailureThreshold is the minimum number of failed keys required
	// before triggering channel-level cascade cooldown. 0 = trigger when ALL
	// keys are in cooldown. Default: 0.
	ChannelFailureThreshold int
}

func (c CooldownConfig) withDefaults() CooldownConfig {
	if c.KeyCooldownDuration == 0 {
		c.KeyCooldownDuration = 30 * time.Second
	}
	if c.ChannelCooldownDuration == 0 {
		c.ChannelCooldownDuration = 5 * time.Minute
	}
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 1
	}
	return c
}

type keyCooldownState struct {
	failures     int
	cooldownUtil time.Time
}

type channelCooldownState struct {
	cooldownUtil time.Time
}

// CascadeCooldownController implements CooldownController with key-level
// cooldown and channel-level cascade cooldown. When all keys in a channel fail,
// the entire channel enters cooldown to prevent thrashing.
type CascadeCooldownController struct {
	Config CooldownConfig

	// KeySource is used to check how many keys a channel has, so we can
	// determine if all keys are in cooldown (triggering cascade).
	KeySource KeySource

	mu       sync.Mutex
	keys     map[string]*keyCooldownState     // keyID → state
	channels map[string]*channelCooldownState  // channelID → state
}

var _ router.CooldownController = (*CascadeCooldownController)(nil)

// NewCascadeCooldownController creates a cascade cooldown controller.
func NewCascadeCooldownController(keySource KeySource, config CooldownConfig) *CascadeCooldownController {
	return &CascadeCooldownController{
		Config:    config.withDefaults(),
		KeySource: keySource,
		keys:      make(map[string]*keyCooldownState),
		channels:  make(map[string]*channelCooldownState),
	}
}

// Allow checks whether the selected channel/key is in cooldown.
func (c *CascadeCooldownController) Allow(_ context.Context, _ *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	if c == nil || selection == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check channel-level cooldown
	channelID := strings.TrimSpace(selection.ChannelID)
	if channelID != "" {
		if cs, ok := c.channels[channelID]; ok && now.Before(cs.cooldownUtil) {
			return fmt.Errorf("channel %s in cooldown until %s", channelID, cs.cooldownUtil.Format(time.RFC3339))
		}
	}

	// Check key-level cooldown
	keyID := strings.TrimSpace(selection.KeyID)
	if keyID != "" {
		if ks, ok := c.keys[keyID]; ok && now.Before(ks.cooldownUtil) {
			return fmt.Errorf("key %s in cooldown until %s", keyID, ks.cooldownUtil.Format(time.RFC3339))
		}
	}

	return nil
}

// RecordResult records a call outcome and triggers cooldown if needed.
func (c *CascadeCooldownController) RecordResult(ctx context.Context, usage *router.ChannelUsageRecord) error {
	if c == nil || usage == nil {
		return nil
	}

	keyID := strings.TrimSpace(usage.KeyID)
	channelID := strings.TrimSpace(usage.ChannelID)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	if usage.Success {
		// On success, reset the key's failure count
		if keyID != "" {
			if ks, ok := c.keys[keyID]; ok {
				ks.failures = 0
			}
		}
		return nil
	}

	// On failure, increment key failure count
	if keyID != "" {
		ks, ok := c.keys[keyID]
		if !ok {
			ks = &keyCooldownState{}
			c.keys[keyID] = ks
		}
		ks.failures++

		if ks.failures >= c.Config.FailureThreshold {
			ks.cooldownUtil = now.Add(c.Config.KeyCooldownDuration)
		}
	}

	// Check cascade: if all keys in the channel are in cooldown, cooldown the channel
	if channelID != "" && c.KeySource != nil {
		c.checkCascadeCooldown(ctx, channelID, now)
	}

	return nil
}

func (c *CascadeCooldownController) checkCascadeCooldown(ctx context.Context, channelID string, now time.Time) {
	// Already in cooldown? Skip.
	if cs, ok := c.channels[channelID]; ok && now.Before(cs.cooldownUtil) {
		return
	}

	keys, err := c.KeySource.ListKeys(ctx, channelID)
	if err != nil || len(keys) == 0 {
		return
	}

	cooledKeys := 0
	for _, key := range keys {
		keyID := strings.TrimSpace(key.ID)
		if keyID == "" {
			continue
		}
		if ks, ok := c.keys[keyID]; ok && now.Before(ks.cooldownUtil) {
			cooledKeys++
		}
	}

	threshold := c.Config.ChannelFailureThreshold
	if threshold <= 0 {
		threshold = len(keys)
	}

	if cooledKeys >= threshold {
		c.channels[channelID] = &channelCooldownState{
			cooldownUtil: now.Add(c.Config.ChannelCooldownDuration),
		}
	}
}

// IsKeyCoolingDown returns whether a key is currently in cooldown.
func (c *CascadeCooldownController) IsKeyCoolingDown(keyID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	ks, ok := c.keys[strings.TrimSpace(keyID)]
	return ok && time.Now().Before(ks.cooldownUtil)
}

// IsChannelCoolingDown returns whether a channel is currently in cooldown.
func (c *CascadeCooldownController) IsChannelCoolingDown(channelID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	cs, ok := c.channels[strings.TrimSpace(channelID)]
	return ok && time.Now().Before(cs.cooldownUtil)
}

// Reset clears all cooldown state.
func (c *CascadeCooldownController) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = make(map[string]*keyCooldownState)
	c.channels = make(map[string]*channelCooldownState)
}
