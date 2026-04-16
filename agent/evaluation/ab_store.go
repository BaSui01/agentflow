package evaluation

import (
	"context"
	"sync"
)

// MemoryExperimentStore 内存实验存储（用于测试和简单场景）
type MemoryExperimentStore struct {
	experiments map[string]*Experiment
	assignments map[string]map[string]string        // experimentID -> userID -> variantID
	results     map[string]map[string][]*EvalResult // experimentID -> variantID -> results
	mu          sync.RWMutex
}

// NewMemoryExperimentStore 创建内存实验存储
func NewMemoryExperimentStore() *MemoryExperimentStore {
	return &MemoryExperimentStore{
		experiments: make(map[string]*Experiment),
		assignments: make(map[string]map[string]string),
		results:     make(map[string]map[string][]*EvalResult),
	}
}

// SaveExperiment 保存实验
func (s *MemoryExperimentStore) SaveExperiment(ctx context.Context, exp *Experiment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 深拷贝实验
	expCopy := *exp
	expCopy.Variants = make([]Variant, len(exp.Variants))
	copy(expCopy.Variants, exp.Variants)

	s.experiments[exp.ID] = &expCopy
	return nil
}

// LoadExperiment 加载实验
func (s *MemoryExperimentStore) LoadExperiment(ctx context.Context, id string) (*Experiment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	exp, ok := s.experiments[id]
	if !ok {
		return nil, ErrExperimentNotFound
	}

	// 返回深拷贝
	expCopy := *exp
	expCopy.Variants = make([]Variant, len(exp.Variants))
	copy(expCopy.Variants, exp.Variants)

	return &expCopy, nil
}

// ListExperiments 列出所有实验
func (s *MemoryExperimentStore) ListExperiments(ctx context.Context) ([]*Experiment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	experiments := make([]*Experiment, 0, len(s.experiments))
	for _, exp := range s.experiments {
		expCopy := *exp
		expCopy.Variants = make([]Variant, len(exp.Variants))
		copy(expCopy.Variants, exp.Variants)
		experiments = append(experiments, &expCopy)
	}

	return experiments, nil
}

// DeleteExperiment 删除实验
func (s *MemoryExperimentStore) DeleteExperiment(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.experiments, id)
	delete(s.assignments, id)
	delete(s.results, id)

	return nil
}

// RecordAssignment 记录分配
func (s *MemoryExperimentStore) RecordAssignment(ctx context.Context, experimentID, userID, variantID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.assignments[experimentID] == nil {
		s.assignments[experimentID] = make(map[string]string)
	}
	s.assignments[experimentID][userID] = variantID

	return nil
}

// GetAssignment 获取用户分配
func (s *MemoryExperimentStore) GetAssignment(ctx context.Context, experimentID, userID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if assignments, ok := s.assignments[experimentID]; ok {
		if variantID, ok := assignments[userID]; ok {
			return variantID, nil
		}
	}

	return "", nil
}

// RecordResult 记录结果
func (s *MemoryExperimentStore) RecordResult(ctx context.Context, experimentID, variantID string, result *EvalResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.results[experimentID] == nil {
		s.results[experimentID] = make(map[string][]*EvalResult)
	}

	// 深拷贝结果
	resultCopy := *result
	if result.Metrics != nil {
		resultCopy.Metrics = make(map[string]float64)
		for k, v := range result.Metrics {
			resultCopy.Metrics[k] = v
		}
	}

	s.results[experimentID][variantID] = append(s.results[experimentID][variantID], &resultCopy)

	return nil
}

// GetResults 获取实验结果
func (s *MemoryExperimentStore) GetResults(ctx context.Context, experimentID string) (map[string][]*EvalResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make(map[string][]*EvalResult)

	if expResults, ok := s.results[experimentID]; ok {
		for variantID, variantResults := range expResults {
			results[variantID] = make([]*EvalResult, len(variantResults))
			for i, r := range variantResults {
				resultCopy := *r
				if r.Metrics != nil {
					resultCopy.Metrics = make(map[string]float64)
					for k, v := range r.Metrics {
						resultCopy.Metrics[k] = v
					}
				}
				results[variantID][i] = &resultCopy
			}
		}
	}

	return results, nil
}

// GetAssignmentCount 获取分配计数（用于测试）
func (s *MemoryExperimentStore) GetAssignmentCount(experimentID string) map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int)
	if assignments, ok := s.assignments[experimentID]; ok {
		for _, variantID := range assignments {
			counts[variantID]++
		}
	}

	return counts
}

// GetResultCount 获取结果计数（用于测试）
func (s *MemoryExperimentStore) GetResultCount(experimentID string) map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int)
	if results, ok := s.results[experimentID]; ok {
		for variantID, variantResults := range results {
			counts[variantID] = len(variantResults)
		}
	}

	return counts
}
