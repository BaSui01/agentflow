// Package conversation provides conversation management with branching and rollback.
package conversation

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
)

// ConversationState represents the state of a conversation at a point in time.
type ConversationState struct {
	ID        string         `json:"id"`
	ParentID  string         `json:"parent_id,omitempty"`
	Messages  []llm.Message  `json:"messages"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Label     string         `json:"label,omitempty"`
}

// Branch represents a conversation branch.
type Branch struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	States      []*ConversationState `json:"states"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	IsActive    bool                 `json:"is_active"`
}

// ConversationTree manages conversation history with branching.
type ConversationTree struct {
	ID           string             `json:"id"`
	RootState    *ConversationState `json:"root_state"`
	Branches     map[string]*Branch `json:"branches"`
	ActiveBranch string             `json:"active_branch"`
	mu           sync.RWMutex
	stateCounter int
}

// NewConversationTree creates a new conversation tree.
func NewConversationTree(id string) *ConversationTree {
	rootState := &ConversationState{
		ID:        "state_0",
		Messages:  []llm.Message{},
		CreatedAt: time.Now(),
		Label:     "root",
	}

	mainBranch := &Branch{
		ID:        "main",
		Name:      "main",
		States:    []*ConversationState{rootState},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsActive:  true,
	}

	return &ConversationTree{
		ID:           id,
		RootState:    rootState,
		Branches:     map[string]*Branch{"main": mainBranch},
		ActiveBranch: "main",
	}
}

// AddMessage adds a message to the active branch.
func (t *ConversationTree) AddMessage(msg llm.Message) *ConversationState {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil {
		return nil
	}

	// Get current state
	currentState := branch.States[len(branch.States)-1]

	// Create new state with the message
	t.stateCounter++
	newState := &ConversationState{
		ID:        fmt.Sprintf("state_%d", t.stateCounter),
		ParentID:  currentState.ID,
		Messages:  append(append([]llm.Message{}, currentState.Messages...), msg),
		CreatedAt: time.Now(),
		Metadata:  make(map[string]any),
	}

	branch.States = append(branch.States, newState)
	branch.UpdatedAt = time.Now()

	return newState
}

// GetCurrentState returns the current state of the active branch.
func (t *ConversationTree) GetCurrentState() *ConversationState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil || len(branch.States) == 0 {
		return nil
	}

	return branch.States[len(branch.States)-1]
}

// GetMessages returns all messages in the current state.
func (t *ConversationTree) GetMessages() []llm.Message {
	state := t.GetCurrentState()
	if state == nil {
		return nil
	}
	return state.Messages
}

// Fork creates a new branch from the current state.
func (t *ConversationTree) Fork(branchName string) (*Branch, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.Branches[branchName]; exists {
		return nil, fmt.Errorf("branch %s already exists", branchName)
	}

	// Get current state
	currentBranch := t.Branches[t.ActiveBranch]
	if currentBranch == nil {
		return nil, fmt.Errorf("no active branch")
	}

	currentState := currentBranch.States[len(currentBranch.States)-1]

	// Create fork state
	t.stateCounter++
	forkState := &ConversationState{
		ID:        fmt.Sprintf("state_%d", t.stateCounter),
		ParentID:  currentState.ID,
		Messages:  append([]llm.Message{}, currentState.Messages...),
		CreatedAt: time.Now(),
		Label:     fmt.Sprintf("fork from %s", t.ActiveBranch),
		Metadata:  make(map[string]any),
	}

	// Create new branch
	newBranch := &Branch{
		ID:          branchName,
		Name:        branchName,
		Description: fmt.Sprintf("Forked from %s at state %s", t.ActiveBranch, currentState.ID),
		States:      []*ConversationState{forkState},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsActive:    false,
	}

	t.Branches[branchName] = newBranch
	return newBranch, nil
}

// SwitchBranch switches to a different branch.
func (t *ConversationTree) SwitchBranch(branchName string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch, exists := t.Branches[branchName]
	if !exists {
		return fmt.Errorf("branch %s not found", branchName)
	}

	// Deactivate current branch
	if currentBranch := t.Branches[t.ActiveBranch]; currentBranch != nil {
		currentBranch.IsActive = false
	}

	// Activate new branch
	branch.IsActive = true
	t.ActiveBranch = branchName

	return nil
}

// Rollback rolls back to a previous state in the current branch.
func (t *ConversationTree) Rollback(stateID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil {
		return fmt.Errorf("no active branch")
	}

	// Find state index
	stateIdx := -1
	for i, state := range branch.States {
		if state.ID == stateID {
			stateIdx = i
			break
		}
	}

	if stateIdx < 0 {
		return fmt.Errorf("state %s not found in branch %s", stateID, t.ActiveBranch)
	}

	// Truncate states after the rollback point
	branch.States = branch.States[:stateIdx+1]
	branch.UpdatedAt = time.Now()

	return nil
}

// RollbackN rolls back N states.
func (t *ConversationTree) RollbackN(n int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil {
		return fmt.Errorf("no active branch")
	}

	if n >= len(branch.States) {
		return fmt.Errorf("cannot rollback %d states, only %d available", n, len(branch.States)-1)
	}

	branch.States = branch.States[:len(branch.States)-n]
	branch.UpdatedAt = time.Now()

	return nil
}

// GetHistory returns the state history of the current branch.
func (t *ConversationTree) GetHistory() []*ConversationState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil {
		return nil
	}

	return branch.States
}

// ListBranches returns all branch names.
func (t *ConversationTree) ListBranches() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	names := make([]string, 0, len(t.Branches))
	for name := range t.Branches {
		names = append(names, name)
	}
	return names
}

// DeleteBranch deletes a branch (cannot delete active or main branch).
func (t *ConversationTree) DeleteBranch(branchName string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if branchName == "main" {
		return fmt.Errorf("cannot delete main branch")
	}

	if branchName == t.ActiveBranch {
		return fmt.Errorf("cannot delete active branch")
	}

	if _, exists := t.Branches[branchName]; !exists {
		return fmt.Errorf("branch %s not found", branchName)
	}

	delete(t.Branches, branchName)
	return nil
}

// MergeBranch merges a branch into the active branch.
func (t *ConversationTree) MergeBranch(sourceBranch string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	source, exists := t.Branches[sourceBranch]
	if !exists {
		return fmt.Errorf("source branch %s not found", sourceBranch)
	}

	target := t.Branches[t.ActiveBranch]
	if target == nil {
		return fmt.Errorf("no active branch")
	}

	// Get messages from source that aren't in target
	targetMsgCount := len(target.States[len(target.States)-1].Messages)
	sourceState := source.States[len(source.States)-1]

	if len(sourceState.Messages) <= targetMsgCount {
		return nil // Nothing to merge
	}

	// Add new messages from source
	newMessages := sourceState.Messages[targetMsgCount:]
	for _, msg := range newMessages {
		t.stateCounter++
		currentState := target.States[len(target.States)-1]
		newState := &ConversationState{
			ID:        fmt.Sprintf("state_%d", t.stateCounter),
			ParentID:  currentState.ID,
			Messages:  append(append([]llm.Message{}, currentState.Messages...), msg),
			CreatedAt: time.Now(),
			Label:     fmt.Sprintf("merged from %s", sourceBranch),
			Metadata:  map[string]any{"merged_from": sourceBranch},
		}
		target.States = append(target.States, newState)
	}

	target.UpdatedAt = time.Now()
	return nil
}

// Export exports the conversation tree to JSON.
func (t *ConversationTree) Export() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return json.Marshal(t)
}

// Import imports a conversation tree from JSON.
func Import(data []byte) (*ConversationTree, error) {
	var tree ConversationTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	return &tree, nil
}

// Snapshot creates a labeled snapshot of the current state.
func (t *ConversationTree) Snapshot(label string) *ConversationState {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil || len(branch.States) == 0 {
		return nil
	}

	currentState := branch.States[len(branch.States)-1]
	currentState.Label = label
	currentState.Metadata["snapshot"] = true
	currentState.Metadata["snapshot_time"] = time.Now().Format(time.RFC3339)

	return currentState
}

// FindSnapshot finds a snapshot by label.
func (t *ConversationTree) FindSnapshot(label string) *ConversationState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, branch := range t.Branches {
		for _, state := range branch.States {
			if state.Label == label {
				return state
			}
		}
	}
	return nil
}

// RestoreSnapshot restores to a labeled snapshot.
func (t *ConversationTree) RestoreSnapshot(label string) error {
	state := t.FindSnapshot(label)
	if state == nil {
		return fmt.Errorf("snapshot %s not found", label)
	}

	// Find which branch contains this state
	t.mu.Lock()
	defer t.mu.Unlock()

	for branchName, branch := range t.Branches {
		for i, s := range branch.States {
			if s.ID == state.ID {
				// Switch to this branch and rollback
				t.ActiveBranch = branchName
				branch.IsActive = true
				branch.States = branch.States[:i+1]
				return nil
			}
		}
	}

	return fmt.Errorf("snapshot state not found in any branch")
}
