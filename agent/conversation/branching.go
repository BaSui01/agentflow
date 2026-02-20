package conversation

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
)

// 对话状态(Conversation State)代表时间点的对话状态.
type ConversationState struct {
	ID        string         `json:"id"`
	ParentID  string         `json:"parent_id,omitempty"`
	Messages  []llm.Message  `json:"messages"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Label     string         `json:"label,omitempty"`
}

// 分会代表谈话分会.
type Branch struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	States      []*ConversationState `json:"states"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	IsActive    bool                 `json:"is_active"`
}

// 对话 树用分支管理对话历史.
type ConversationTree struct {
	ID           string             `json:"id"`
	RootState    *ConversationState `json:"root_state"`
	Branches     map[string]*Branch `json:"branches"`
	ActiveBranch string             `json:"active_branch"`
	mu           sync.RWMutex
	stateCounter int
}

// 新建组合 树创造出一棵新的对话树.
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

// 添加 Message 为活动分支添加了消息 。
func (t *ConversationTree) AddMessage(msg llm.Message) *ConversationState {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil {
		return nil
	}

	// 获取当前状态
	currentState := branch.States[len(branch.States)-1]

	// 用信件创建新状态
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

// GetCurentState 返回活动分支当前状态 。
func (t *ConversationTree) GetCurrentState() *ConversationState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil || len(branch.States) == 0 {
		return nil
	}

	return branch.States[len(branch.States)-1]
}

// GetMessages 返回当前状态下的所有信件 。
func (t *ConversationTree) GetMessages() []llm.Message {
	state := t.GetCurrentState()
	if state == nil {
		return nil
	}
	return state.Messages
}

// 叉从当前状态创建出一个新的分支.
func (t *ConversationTree) Fork(branchName string) (*Branch, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.Branches[branchName]; exists {
		return nil, fmt.Errorf("branch %s already exists", branchName)
	}

	// 获取当前状态
	currentBranch := t.Branches[t.ActiveBranch]
	if currentBranch == nil {
		return nil, fmt.Errorf("no active branch")
	}

	currentState := currentBranch.States[len(currentBranch.States)-1]

	// 创建分叉状态
	t.stateCounter++
	forkState := &ConversationState{
		ID:        fmt.Sprintf("state_%d", t.stateCounter),
		ParentID:  currentState.ID,
		Messages:  append([]llm.Message{}, currentState.Messages...),
		CreatedAt: time.Now(),
		Label:     fmt.Sprintf("fork from %s", t.ActiveBranch),
		Metadata:  make(map[string]any),
	}

	// 创建新分支
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

// 切换Branch切换到不同的分支.
func (t *ConversationTree) SwitchBranch(branchName string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch, exists := t.Branches[branchName]
	if !exists {
		return fmt.Errorf("branch %s not found", branchName)
	}

	// 取消当前分支
	if currentBranch := t.Branches[t.ActiveBranch]; currentBranch != nil {
		currentBranch.IsActive = false
	}

	// 启用新分支
	branch.IsActive = true
	t.ActiveBranch = branchName

	return nil
}

// 后滚回当前分行的上个状态.
func (t *ConversationTree) Rollback(stateID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil {
		return fmt.Errorf("no active branch")
	}

	// 查找状态索引
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

	// 倒转点后断线状态
	branch.States = branch.States[:stateIdx+1]
	branch.UpdatedAt = time.Now()

	return nil
}

// 滚回N 滚回N州。
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

// GetHistory还原了目前分行的州史.
func (t *ConversationTree) GetHistory() []*ConversationState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	branch := t.Branches[t.ActiveBranch]
	if branch == nil {
		return nil
	}

	return branch.States
}

// ListBranches 返回所有分支名称 。
func (t *ConversationTree) ListBranches() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	names := make([]string, 0, len(t.Branches))
	for name := range t.Branches {
		names = append(names, name)
	}
	return names
}

// 删除Branch删除一个分支(不能删除活动分支或主分支).
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

// 合并Branch将一个分支合并到活动分支.
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

	// 从没有目标的来源获取消息
	targetMsgCount := len(target.States[len(target.States)-1].Messages)
	sourceState := source.States[len(source.States)-1]

	if len(sourceState.Messages) <= targetMsgCount {
		return nil // Nothing to merge
	}

	// 从源添加新信件
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

// 导出对话树给 JSON 。
func (t *ConversationTree) Export() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return json.Marshal(t)
}

// 导入 JSON 的谈话树 。
func Import(data []byte) (*ConversationTree, error) {
	var tree ConversationTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	return &tree, nil
}

// 抓图创建当前状态的标签快照.
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

// FindSnapshot通过标签找到快照.
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

// 还原Snapshot恢复到标签快照.
func (t *ConversationTree) RestoreSnapshot(label string) error {
	state := t.FindSnapshot(label)
	if state == nil {
		return fmt.Errorf("snapshot %s not found", label)
	}

	// 查找哪个分支包含此状态
	t.mu.Lock()
	defer t.mu.Unlock()

	for branchName, branch := range t.Branches {
		for i, s := range branch.States {
			if s.ID == state.ID {
				// 切换到此分支并回滚
				t.ActiveBranch = branchName
				branch.IsActive = true
				branch.States = branch.States[:i+1]
				return nil
			}
		}
	}

	return fmt.Errorf("snapshot state not found in any branch")
}
