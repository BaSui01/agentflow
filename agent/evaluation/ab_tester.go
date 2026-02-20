// 成套评价为AI代理提供了自动化的评价框架.
package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// A/B 测试相关错误
var (
	ErrExperimentNotFound  = errors.New("experiment not found")
	ErrExperimentNotActive = errors.New("experiment not active")
	ErrNoVariants          = errors.New("no variants defined")
	ErrInvalidWeights      = errors.New("invalid variant weights")
	ErrVariantNotFound     = errors.New("variant not found")
)

// ExperimentStatus 实验状态
type ExperimentStatus string

const (
	ExperimentStatusDraft    ExperimentStatus = "draft"
	ExperimentStatusRunning  ExperimentStatus = "running"
	ExperimentStatusPaused   ExperimentStatus = "paused"
	ExperimentStatusComplete ExperimentStatus = "completed"
)

// Variant 实验变体
// 审定:所需经费11.1、11.5
type Variant struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Config    map[string]any `json:"config"`
	Weight    float64        `json:"weight"` // 流量权重
	IsControl bool           `json:"is_control"`
}

// Experiment 实验定义
// 审定:所需经费11.1
type Experiment struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Variants    []Variant        `json:"variants"`
	Metrics     []string         `json:"metrics"`
	StartTime   time.Time        `json:"start_time"`
	EndTime     *time.Time       `json:"end_time,omitempty"`
	Status      ExperimentStatus `json:"status"`
}

// VariantResult 变体结果
// 核证:所需经费 11.3
type VariantResult struct {
	VariantID   string             `json:"variant_id"`
	SampleCount int                `json:"sample_count"`
	Metrics     map[string]float64 `json:"metrics"`
	StdDev      map[string]float64 `json:"std_dev"`
	// 原始数据用于统计分析
	rawMetrics map[string][]float64
}

// ExperimentResult 实验结果
// 审定: 所需经费 11.3, 11.4
type ExperimentResult struct {
	ExperimentID   string                    `json:"experiment_id"`
	VariantResults map[string]*VariantResult `json:"variant_results"`
	Winner         string                    `json:"winner,omitempty"`
	Confidence     float64                   `json:"confidence"`
	SampleSize     int                       `json:"sample_size"`
	Duration       time.Duration             `json:"duration"`
}

// ExperimentStore 实验存储接口
type ExperimentStore interface {
	// SaveExperiment 保存实验
	SaveExperiment(ctx context.Context, exp *Experiment) error
	// LoadExperiment 加载实验
	LoadExperiment(ctx context.Context, id string) (*Experiment, error)
	// ListExperiments 列出所有实验
	ListExperiments(ctx context.Context) ([]*Experiment, error)
	// DeleteExperiment 删除实验
	DeleteExperiment(ctx context.Context, id string) error
	// RecordAssignment 记录分配
	RecordAssignment(ctx context.Context, experimentID, userID, variantID string) error
	// GetAssignment 获取用户分配
	GetAssignment(ctx context.Context, experimentID, userID string) (string, error)
	// RecordResult 记录结果
	RecordResult(ctx context.Context, experimentID, variantID string, result *EvalResult) error
	// GetResults 获取实验结果
	GetResults(ctx context.Context, experimentID string) (map[string][]*EvalResult, error)
}

// ABTester A/B 测试器
// 11.1、11.2、11.3、11.5
type ABTester struct {
	experiments map[string]*Experiment
	store       ExperimentStore
	logger      *zap.Logger
	mu          sync.RWMutex
}

// NewABTester 创建 A/B 测试器
func NewABTester(store ExperimentStore, logger *zap.Logger) *ABTester {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ABTester{
		experiments: make(map[string]*Experiment),
		store:       store,
		logger:      logger,
	}
}

// CreateExperiment 创建实验
// 审定:所需经费11.1
func (t *ABTester) CreateExperiment(exp *Experiment) error {
	if exp == nil {
		return errors.New("experiment cannot be nil")
	}
	if exp.ID == "" {
		return errors.New("experiment ID is required")
	}
	if len(exp.Variants) == 0 {
		return ErrNoVariants
	}

	// 验证权重
	if err := t.validateWeights(exp.Variants); err != nil {
		return err
	}

	// 设置默认状态
	if exp.Status == "" {
		exp.Status = ExperimentStatusDraft
	}

	t.mu.Lock()
	t.experiments[exp.ID] = exp
	t.mu.Unlock()

	// 持久化到存储
	if t.store != nil {
		if err := t.store.SaveExperiment(context.Background(), exp); err != nil {
			t.logger.Warn("failed to save experiment to store", zap.Error(err))
		}
	}

	t.logger.Info("experiment created",
		zap.String("id", exp.ID),
		zap.String("name", exp.Name),
		zap.Int("variants", len(exp.Variants)))

	return nil
}

// validateWeights 验证变体权重
func (t *ABTester) validateWeights(variants []Variant) error {
	var totalWeight float64
	for _, v := range variants {
		if v.Weight < 0 {
			return fmt.Errorf("%w: variant %s has negative weight", ErrInvalidWeights, v.ID)
		}
		totalWeight += v.Weight
	}
	if totalWeight <= 0 {
		return fmt.Errorf("%w: total weight must be positive", ErrInvalidWeights)
	}
	return nil
}

// GetExperiment 获取实验
func (t *ABTester) GetExperiment(experimentID string) (*Experiment, error) {
	t.mu.RLock()
	exp, ok := t.experiments[experimentID]
	t.mu.RUnlock()

	if ok {
		return exp, nil
	}

	// 尝试从存储加载
	if t.store != nil {
		exp, err := t.store.LoadExperiment(context.Background(), experimentID)
		if err != nil {
			return nil, ErrExperimentNotFound
		}
		t.mu.Lock()
		t.experiments[experimentID] = exp
		t.mu.Unlock()
		return exp, nil
	}

	return nil, ErrExperimentNotFound
}

// StartExperiment 启动实验
func (t *ABTester) StartExperiment(experimentID string) error {
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return err
	}

	t.mu.Lock()
	exp.Status = ExperimentStatusRunning
	exp.StartTime = time.Now()
	t.mu.Unlock()

	if t.store != nil {
		if err := t.store.SaveExperiment(context.Background(), exp); err != nil {
			t.logger.Warn("failed to save experiment status", zap.Error(err))
		}
	}

	t.logger.Info("experiment started", zap.String("id", experimentID))
	return nil
}

// PauseExperiment 暂停实验
func (t *ABTester) PauseExperiment(experimentID string) error {
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return err
	}

	t.mu.Lock()
	exp.Status = ExperimentStatusPaused
	t.mu.Unlock()

	if t.store != nil {
		if err := t.store.SaveExperiment(context.Background(), exp); err != nil {
			t.logger.Warn("failed to save experiment status", zap.Error(err))
		}
	}

	t.logger.Info("experiment paused", zap.String("id", experimentID))
	return nil
}

// CompleteExperiment 完成实验
func (t *ABTester) CompleteExperiment(experimentID string) error {
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return err
	}

	t.mu.Lock()
	exp.Status = ExperimentStatusComplete
	now := time.Now()
	exp.EndTime = &now
	t.mu.Unlock()

	if t.store != nil {
		if err := t.store.SaveExperiment(context.Background(), exp); err != nil {
			t.logger.Warn("failed to save experiment status", zap.Error(err))
		}
	}

	t.logger.Info("experiment completed", zap.String("id", experimentID))
	return nil
}

// Assign 分配变体
// 使用一致性哈希确保同一用户始终分配到同一变体
// 审定:所需经费11.2
func (t *ABTester) Assign(experimentID, userID string) (*Variant, error) {
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return nil, err
	}

	if exp.Status != ExperimentStatusRunning {
		return nil, ErrExperimentNotActive
	}

	// 检查是否已有分配
	if t.store != nil {
		if variantID, err := t.store.GetAssignment(context.Background(), experimentID, userID); err == nil && variantID != "" {
			for i := range exp.Variants {
				if exp.Variants[i].ID == variantID {
					return &exp.Variants[i], nil
				}
			}
		}
	}

	// 使用一致性哈希分配变体
	variant := t.assignByHash(exp.Variants, experimentID, userID)

	// 记录分配
	if t.store != nil {
		if err := t.store.RecordAssignment(context.Background(), experimentID, userID, variant.ID); err != nil {
			t.logger.Warn("failed to record assignment", zap.Error(err))
		}
	}

	t.logger.Debug("variant assigned",
		zap.String("experiment", experimentID),
		zap.String("user", userID),
		zap.String("variant", variant.ID))

	return variant, nil
}

// assignByHash 使用哈希进行流量分配
// 审定:所需经费11.2
func (t *ABTester) assignByHash(variants []Variant, experimentID, userID string) *Variant {
	// 计算哈希值
	hash := sha256.Sum256([]byte(experimentID + ":" + userID))
	hashValue := binary.BigEndian.Uint64(hash[:8])

	// 归一化到 [0, 1)
	normalizedHash := float64(hashValue) / float64(^uint64(0))

	// 计算总权重
	var totalWeight float64
	for _, v := range variants {
		totalWeight += v.Weight
	}

	// 按权重分配
	var cumulative float64
	threshold := normalizedHash * totalWeight
	for i := range variants {
		cumulative += variants[i].Weight
		if threshold < cumulative {
			return &variants[i]
		}
	}

	// 默认返回最后一个变体
	return &variants[len(variants)-1]
}

// RecordResult 记录结果
// 核证:所需经费 11.3
func (t *ABTester) RecordResult(experimentID, variantID string, result *EvalResult) error {
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return err
	}

	// 验证变体存在
	found := false
	for _, v := range exp.Variants {
		if v.ID == variantID {
			found = true
			break
		}
	}
	if !found {
		return ErrVariantNotFound
	}

	// 持久化结果
	if t.store != nil {
		if err := t.store.RecordResult(context.Background(), experimentID, variantID, result); err != nil {
			return fmt.Errorf("failed to record result: %w", err)
		}
	}

	t.logger.Debug("result recorded",
		zap.String("experiment", experimentID),
		zap.String("variant", variantID),
		zap.Float64("score", result.Score))

	return nil
}

// Analyze 分析实验结果
// 审定: 所需经费 11.3, 11.4
func (t *ABTester) Analyze(ctx context.Context, experimentID string) (*ExperimentResult, error) {
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return nil, err
	}

	// 获取所有结果
	var allResults map[string][]*EvalResult
	if t.store != nil {
		allResults, err = t.store.GetResults(ctx, experimentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get results: %w", err)
		}
	} else {
		allResults = make(map[string][]*EvalResult)
	}

	// 计算每个变体的统计数据
	result := &ExperimentResult{
		ExperimentID:   experimentID,
		VariantResults: make(map[string]*VariantResult),
	}

	for _, variant := range exp.Variants {
		variantResults := allResults[variant.ID]
		vr := t.calculateVariantStats(variant.ID, variantResults, exp.Metrics)
		result.VariantResults[variant.ID] = vr
		result.SampleSize += vr.SampleCount
	}

	// 计算实验持续时间
	if exp.EndTime != nil {
		result.Duration = exp.EndTime.Sub(exp.StartTime)
	} else {
		result.Duration = time.Since(exp.StartTime)
	}

	// 确定获胜者和置信度
	t.determineWinner(result, exp)

	return result, nil
}

// calculateVariantStats 计算变体统计数据
func (t *ABTester) calculateVariantStats(variantID string, results []*EvalResult, metrics []string) *VariantResult {
	vr := &VariantResult{
		VariantID:   variantID,
		SampleCount: len(results),
		Metrics:     make(map[string]float64),
		StdDev:      make(map[string]float64),
		rawMetrics:  make(map[string][]float64),
	}

	if len(results) == 0 {
		return vr
	}

	// 收集所有指标值
	for _, r := range results {
		// 收集 score
		vr.rawMetrics["score"] = append(vr.rawMetrics["score"], r.Score)

		// 收集其他指标
		for _, m := range metrics {
			if val, ok := r.Metrics[m]; ok {
				vr.rawMetrics[m] = append(vr.rawMetrics[m], val)
			}
		}
	}

	// 计算均值和标准差
	for metric, values := range vr.rawMetrics {
		if len(values) == 0 {
			continue
		}
		mean := calculateMean(values)
		vr.Metrics[metric] = mean
		vr.StdDev[metric] = calculateStdDeviation(values, mean)
	}

	return vr
}

// determineWinner 确定获胜者
// 审定:所需经费 11.4
func (t *ABTester) determineWinner(result *ExperimentResult, exp *Experiment) {
	if len(result.VariantResults) < 2 {
		return
	}

	// 找到对照组
	var controlID string
	for _, v := range exp.Variants {
		if v.IsControl {
			controlID = v.ID
			break
		}
	}

	// 如果没有对照组，使用第一个变体
	if controlID == "" && len(exp.Variants) > 0 {
		controlID = exp.Variants[0].ID
	}

	controlResult := result.VariantResults[controlID]
	if controlResult == nil || controlResult.SampleCount == 0 {
		return
	}

	// 比较所有变体与对照组
	var bestVariant string
	var bestImprovement float64
	var bestConfidence float64

	for variantID, vr := range result.VariantResults {
		if variantID == controlID {
			continue
		}
		if vr.SampleCount == 0 {
			continue
		}

		// 使用 score 指标进行比较
		controlScore := controlResult.Metrics["score"]
		variantScore := vr.Metrics["score"]

		// 计算改进幅度
		improvement := variantScore - controlScore

		// 计算统计显著性 (使用 t-test)
		confidence := t.calculateConfidence(
			controlResult.rawMetrics["score"],
			vr.rawMetrics["score"],
		)

		if improvement > bestImprovement && confidence > 0.95 {
			bestVariant = variantID
			bestImprovement = improvement
			bestConfidence = confidence
		}
	}

	if bestVariant != "" {
		result.Winner = bestVariant
		result.Confidence = bestConfidence
	}
}

// calculateConfidence 计算统计置信度 (使用 Welch's t-test)
func (t *ABTester) calculateConfidence(control, treatment []float64) float64 {
	if len(control) < 2 || len(treatment) < 2 {
		return 0
	}

	// 计算均值
	meanControl := calculateMean(control)
	meanTreatment := calculateMean(treatment)

	// 计算方差
	varControl := calculateVariance(control, meanControl)
	varTreatment := calculateVariance(treatment, meanTreatment)

	// 计算 t 统计量
	n1 := float64(len(control))
	n2 := float64(len(treatment))

	se := math.Sqrt(varControl/n1 + varTreatment/n2)
	if se == 0 {
		return 0
	}

	tStat := math.Abs(meanTreatment-meanControl) / se

	// 计算自由度 (Welch-Satterthwaite)
	v1 := varControl / n1
	v2 := varTreatment / n2
	df := (v1 + v2) * (v1 + v2) / (v1*v1/(n1-1) + v2*v2/(n2-1))

	// 使用近似的 t 分布 CDF 计算 p 值
	pValue := tDistributionPValue(tStat, df)

	// 返回置信度 (1 - p-value)
	return 1 - pValue
}

// ListExperiments 列出所有实验
func (t *ABTester) ListExperiments() []*Experiment {
	t.mu.RLock()
	defer t.mu.RUnlock()

	experiments := make([]*Experiment, 0, len(t.experiments))
	for _, exp := range t.experiments {
		experiments = append(experiments, exp)
	}

	// 按创建时间排序
	sort.Slice(experiments, func(i, j int) bool {
		return experiments[i].StartTime.Before(experiments[j].StartTime)
	})

	return experiments
}

// DeleteExperiment 删除实验
func (t *ABTester) DeleteExperiment(experimentID string) error {
	t.mu.Lock()
	delete(t.experiments, experimentID)
	t.mu.Unlock()

	if t.store != nil {
		if err := t.store.DeleteExperiment(context.Background(), experimentID); err != nil {
			return fmt.Errorf("failed to delete experiment from store: %w", err)
		}
	}

	t.logger.Info("experiment deleted", zap.String("id", experimentID))
	return nil
}

// 自动选择Winner 自动选择获胜的变量配置
// 当检测到统计意义时。
// 核实:所需经费 11.6
func (t *ABTester) AutoSelectWinner(ctx context.Context, experimentID string, minConfidence float64) (*Variant, error) {
	if minConfidence <= 0 || minConfidence > 1 {
		minConfidence = 0.95 // Default 95% confidence
	}

	// 分析实验
	result, err := t.Analyze(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze experiment: %w", err)
	}

	// 看看有没有赢家有足够的自信
	if result.Winner == "" {
		return nil, fmt.Errorf("no statistically significant winner detected")
	}

	if result.Confidence < minConfidence {
		return nil, fmt.Errorf("confidence %.2f%% is below threshold %.2f%%",
			result.Confidence*100, minConfidence*100)
	}

	// 让实验找到胜利的变体
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return nil, err
	}

	// 查找并返回获胜的变体
	for i := range exp.Variants {
		if exp.Variants[i].ID == result.Winner {
			t.logger.Info("auto-selected winner",
				zap.String("experiment", experimentID),
				zap.String("winner", result.Winner),
				zap.Float64("confidence", result.Confidence))
			return &exp.Variants[i], nil
		}
	}

	return nil, ErrVariantNotFound
}

// 统计报告是详细的统计分析报告
// 审定:所需经费 11.4
type StatisticalReport struct {
	ExperimentID     string                    `json:"experiment_id"`
	ExperimentName   string                    `json:"experiment_name"`
	Status           ExperimentStatus          `json:"status"`
	Duration         time.Duration             `json:"duration"`
	TotalSamples     int                       `json:"total_samples"`
	VariantReports   map[string]*VariantReport `json:"variant_reports"`
	Comparisons      []*VariantComparison      `json:"comparisons"`
	Winner           string                    `json:"winner,omitempty"`
	WinnerConfidence float64                   `json:"winner_confidence,omitempty"`
	Recommendation   string                    `json:"recommendation"`
	GeneratedAt      time.Time                 `json:"generated_at"`
}

// 变量报告载有单一变量的详细统计数据
type VariantReport struct {
	VariantID    string                `json:"variant_id"`
	VariantName  string                `json:"variant_name"`
	IsControl    bool                  `json:"is_control"`
	SampleCount  int                   `json:"sample_count"`
	Metrics      map[string]float64    `json:"metrics"`
	StdDev       map[string]float64    `json:"std_dev"`
	ConfInterval map[string][2]float64 `json:"confidence_interval"` // 95% CI
}

// 变量比较包含两个变量之间的比较结果
type VariantComparison struct {
	ControlID      string             `json:"control_id"`
	TreatmentID    string             `json:"treatment_id"`
	MetricDeltas   map[string]float64 `json:"metric_deltas"`   // treatment - control
	RelativeChange map[string]float64 `json:"relative_change"` // percentage change
	PValues        map[string]float64 `json:"p_values"`
	Confidence     map[string]float64 `json:"confidence"`
	Significant    map[string]bool    `json:"significant"` // at 95% level
}

// 生成报告生成一份全面的统计意义分析报告
// 审定:所需经费 11.4
func (t *ABTester) GenerateReport(ctx context.Context, experimentID string) (*StatisticalReport, error) {
	exp, err := t.GetExperiment(experimentID)
	if err != nil {
		return nil, err
	}

	// 获取分析结果
	result, err := t.Analyze(ctx, experimentID)
	if err != nil {
		return nil, err
	}

	report := &StatisticalReport{
		ExperimentID:     experimentID,
		ExperimentName:   exp.Name,
		Status:           exp.Status,
		Duration:         result.Duration,
		TotalSamples:     result.SampleSize,
		VariantReports:   make(map[string]*VariantReport),
		Comparisons:      make([]*VariantComparison, 0),
		Winner:           result.Winner,
		WinnerConfidence: result.Confidence,
		GeneratedAt:      time.Now(),
	}

	// 生成变量报告
	for _, variant := range exp.Variants {
		vr := result.VariantResults[variant.ID]
		if vr == nil {
			continue
		}

		variantReport := &VariantReport{
			VariantID:    variant.ID,
			VariantName:  variant.Name,
			IsControl:    variant.IsControl,
			SampleCount:  vr.SampleCount,
			Metrics:      vr.Metrics,
			StdDev:       vr.StdDev,
			ConfInterval: make(map[string][2]float64),
		}

		// 计算95%的置信间隔
		for metric, mean := range vr.Metrics {
			stdDev := vr.StdDev[metric]
			n := float64(vr.SampleCount)
			if n > 1 {
				// 95% CI: 平均值± 1.96 * (stdDev / sqrt(n))
				margin := 1.96 * stdDev / math.Sqrt(n)
				variantReport.ConfInterval[metric] = [2]float64{mean - margin, mean + margin}
			}
		}

		report.VariantReports[variant.ID] = variantReport
	}

	// 查找控制变体
	var controlID string
	for _, v := range exp.Variants {
		if v.IsControl {
			controlID = v.ID
			break
		}
	}
	if controlID == "" && len(exp.Variants) > 0 {
		controlID = exp.Variants[0].ID
	}

	controlResult := result.VariantResults[controlID]
	if controlResult == nil || controlResult.SampleCount == 0 {
		report.Recommendation = "Insufficient data for control group"
		return report, nil
	}

	// 生成控件和每种处理方法的比较
	for _, variant := range exp.Variants {
		if variant.ID == controlID {
			continue
		}

		treatmentResult := result.VariantResults[variant.ID]
		if treatmentResult == nil || treatmentResult.SampleCount == 0 {
			continue
		}

		comparison := &VariantComparison{
			ControlID:      controlID,
			TreatmentID:    variant.ID,
			MetricDeltas:   make(map[string]float64),
			RelativeChange: make(map[string]float64),
			PValues:        make(map[string]float64),
			Confidence:     make(map[string]float64),
			Significant:    make(map[string]bool),
		}

		// 比较每个度量
		for metric := range controlResult.Metrics {
			controlMean := controlResult.Metrics[metric]
			treatmentMean := treatmentResult.Metrics[metric]

			// 计算三角形
			delta := treatmentMean - controlMean
			comparison.MetricDeltas[metric] = delta

			// 计算相对变化
			if controlMean != 0 {
				comparison.RelativeChange[metric] = (delta / controlMean) * 100
			}

			// 计算统计意义
			controlData := controlResult.rawMetrics[metric]
			treatmentData := treatmentResult.rawMetrics[metric]

			if len(controlData) >= 2 && len(treatmentData) >= 2 {
				confidence := t.calculateConfidence(controlData, treatmentData)
				pValue := 1 - confidence
				comparison.PValues[metric] = pValue
				comparison.Confidence[metric] = confidence
				comparison.Significant[metric] = confidence >= 0.95
			}
		}

		report.Comparisons = append(report.Comparisons, comparison)
	}

	// 生成建议
	report.Recommendation = t.generateRecommendation(report)

	return report, nil
}

// 生成基于分析的建议
func (t *ABTester) generateRecommendation(report *StatisticalReport) string {
	if report.TotalSamples < 100 {
		return "Insufficient sample size. Continue collecting data for reliable results."
	}

	if report.Winner != "" && report.WinnerConfidence >= 0.95 {
		return fmt.Sprintf("Recommend adopting variant '%s' with %.1f%% confidence.",
			report.Winner, report.WinnerConfidence*100)
	}

	if report.Winner != "" && report.WinnerConfidence >= 0.90 {
		return fmt.Sprintf("Variant '%s' shows promise (%.1f%% confidence). Consider collecting more data.",
			report.Winner, report.WinnerConfidence*100)
	}

	// 检查是否有任何比较显示意义
	for _, comp := range report.Comparisons {
		for metric, sig := range comp.Significant {
			if sig {
				return fmt.Sprintf("Significant difference detected in '%s' metric. Review detailed comparison.", metric)
			}
		}
	}

	return "No statistically significant difference detected. Consider continuing the experiment or reviewing hypothesis."
}

// 辅助功能

func calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateStdDeviation(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	return math.Sqrt(calculateVariance(values, mean))
}

func calculateVariance(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sumSquares float64
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	return sumSquares / float64(len(values)-1) // 使用 n-1 (样本方差)
}

// tDistributionPValue 计算 t 分布的双尾 p 值 (近似)
func tDistributionPValue(t, df float64) float64 {
	// 使用近似公式计算 t 分布 CDF
	// 对于大自由度，t 分布接近正态分布
	if df > 100 {
		// 使用正态分布近似
		return 2 * (1 - normalCDF(t))
	}

	// 使用 Beta 函数近似
	x := df / (df + t*t)
	return incompleteBeta(df/2, 0.5, x)
}

// normalCDF 标准正态分布 CDF (近似)
func normalCDF(x float64) float64 {
	// 使用 Abramowitz and Stegun 近似
	const (
		a1 = 0.254829592
		a2 = -0.284496736
		a3 = 1.421413741
		a4 = -1.453152027
		a5 = 1.061405429
		p  = 0.3275911
	)

	sign := 1.0
	if x < 0 {
		sign = -1.0
	}
	x = math.Abs(x) / math.Sqrt(2)

	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)

	return 0.5 * (1.0 + sign*y)
}

// incompleteBeta 不完全 Beta 函数 (近似)
func incompleteBeta(a, b, x float64) float64 {
	// 使用连分数展开近似
	if x == 0 || x == 1 {
		return x
	}

	// 简化近似
	bt := math.Exp(
		lgamma(a+b) - lgamma(a) - lgamma(b) +
			a*math.Log(x) + b*math.Log(1-x),
	)

	if x < (a+1)/(a+b+2) {
		return bt * betaCF(a, b, x) / a
	}
	return 1 - bt*betaCF(b, a, 1-x)/b
}

// betaCF Beta 函数的连分数展开
func betaCF(a, b, x float64) float64 {
	const maxIterations = 100
	const epsilon = 1e-10

	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < epsilon {
		d = epsilon
	}
	d = 1 / d
	h := d

	for m := 1; m <= maxIterations; m++ {
		m2 := 2 * m
		aa := float64(m) * (b - float64(m)) * x / ((qam + float64(m2)) * (a + float64(m2)))
		d = 1 + aa*d
		if math.Abs(d) < epsilon {
			d = epsilon
		}
		c = 1 + aa/c
		if math.Abs(c) < epsilon {
			c = epsilon
		}
		d = 1 / d
		h *= d * c

		aa = -(a + float64(m)) * (qab + float64(m)) * x / ((a + float64(m2)) * (qap + float64(m2)))
		d = 1 + aa*d
		if math.Abs(d) < epsilon {
			d = epsilon
		}
		c = 1 + aa/c
		if math.Abs(c) < epsilon {
			c = epsilon
		}
		d = 1 / d
		del := d * c
		h *= del

		if math.Abs(del-1) < epsilon {
			break
		}
	}

	return h
}

// lgamma 对数 Gamma 函数
func lgamma(x float64) float64 {
	result, _ := math.Lgamma(x)
	return result
}
