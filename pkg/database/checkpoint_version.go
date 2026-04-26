// pkg/database/checkpoint_version.go

package database

import (
	"encoding/json"
	"sort"
	"time"
)

// CheckpointVersion 表示检查点版本的元数据
// 用于版本列表查询，避免加载完整检查点数据
type CheckpointVersion struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	State     string    `json:"state,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	ThreadID  string    `json:"thread_id,omitempty"`
}

// CheckpointVersionList 检查点版本列表
type CheckpointVersionList struct {
	ThreadID string              `json:"thread_id"`
	Versions []CheckpointVersion `json:"versions"`
	Total    int                 `json:"total"`
}

// CleanupResult 清理操作结果
type CleanupResult struct {
	DeletedCount int64 `json:"deleted_count"`
	FreedBytes   int64 `json:"freed_bytes,omitempty"`
}

// ToJSON 将 CheckpointVersion 序列化为 JSON 字节
func (cv *CheckpointVersion) ToJSON() ([]byte, error) {
	return json.Marshal(cv)
}

// FromJSON 从 JSON 字节反序列化到 CheckpointVersion
func (cv *CheckpointVersion) FromJSON(data []byte) error {
	return json.Unmarshal(data, cv)
}

// ToJSON 将 CheckpointVersionList 序列化为 JSON 字节
func (cvl *CheckpointVersionList) ToJSON() ([]byte, error) {
	return json.Marshal(cvl)
}

// FromJSON 从 JSON 字节反序列化到 CheckpointVersionList
func (cvl *CheckpointVersionList) FromJSON(data []byte) error {
	return json.Unmarshal(data, cvl)
}

// ToJSON 将 CleanupResult 序列化为 JSON 字节
func (cr *CleanupResult) ToJSON() ([]byte, error) {
	return json.Marshal(cr)
}

// FromJSON 从 JSON 字节反序列化到 CleanupResult
func (cr *CleanupResult) FromJSON(data []byte) error {
	return json.Unmarshal(data, cr)
}

// IsZero 检查 CheckpointVersion 是否为零值
func (cv *CheckpointVersion) IsZero() bool {
	return cv.ID == "" && cv.Version == 0 && cv.CreatedAt.IsZero()
}

// Equal 检查两个 CheckpointVersion 是否相等
func (cv *CheckpointVersion) Equal(other *CheckpointVersion) bool {
	if cv == nil || other == nil {
		return cv == other
	}
	return cv.ID == other.ID &&
		cv.Version == other.Version &&
		cv.CreatedAt.Equal(other.CreatedAt) &&
		cv.State == other.State &&
		cv.Summary == other.Summary &&
		cv.ThreadID == other.ThreadID
}

// Before 比较两个检查点版本的创建时间
// 用于按时间排序
func (cv *CheckpointVersion) Before(other *CheckpointVersion) bool {
	if cv == nil || other == nil {
		return false
	}
	return cv.CreatedAt.Before(other.CreatedAt)
}

// After 比较两个检查点版本的创建时间
// 用于按时间排序
func (cv *CheckpointVersion) After(other *CheckpointVersion) bool {
	if cv == nil || other == nil {
		return false
	}
	return cv.CreatedAt.After(other.CreatedAt)
}

// CompareByVersion 比较两个检查点的版本号
// 返回 -1, 0, 1 分别表示小于、等于、大于
func (cv *CheckpointVersion) CompareByVersion(other *CheckpointVersion) int {
	if cv.Version < other.Version {
		return -1
	} else if cv.Version > other.Version {
		return 1
	}
	return 0
}

// IsZero 检查 CheckpointVersionList 是否为零值
func (cvl *CheckpointVersionList) IsZero() bool {
	return cvl.ThreadID == "" && len(cvl.Versions) == 0 && cvl.Total == 0
}

// SortByCreatedAt 按创建时间排序版本列表
// desc 为 true 时降序，false 时升序
func (cvl *CheckpointVersionList) SortByCreatedAt(desc bool) {
	sort.Slice(cvl.Versions, func(i, j int) bool {
		if desc {
			return cvl.Versions[i].CreatedAt.After(cvl.Versions[j].CreatedAt)
		}
		return cvl.Versions[i].CreatedAt.Before(cvl.Versions[j].CreatedAt)
	})
}

// SortByVersion 按版本号排序版本列表
// desc 为 true 时降序，false 时升序
func (cvl *CheckpointVersionList) SortByVersion(desc bool) {
	sort.Slice(cvl.Versions, func(i, j int) bool {
		if desc {
			return cvl.Versions[i].Version > cvl.Versions[j].Version
		}
		return cvl.Versions[i].Version < cvl.Versions[j].Version
	})
}

// Latest 获取最新版本的检查点
// 按版本号降序排序后返回第一个
func (cvl *CheckpointVersionList) Latest() *CheckpointVersion {
	if len(cvl.Versions) == 0 {
		return nil
	}

	// 找到版本号最大的
	latest := &cvl.Versions[0]
	for i := 1; i < len(cvl.Versions); i++ {
		if cvl.Versions[i].Version > latest.Version {
			latest = &cvl.Versions[i]
		}
	}
	return latest
}

// Oldest 获取最旧版本的检查点
// 按版本号升序排序后返回第一个
func (cvl *CheckpointVersionList) Oldest() *CheckpointVersion {
	if len(cvl.Versions) == 0 {
		return nil
	}

	// 找到版本号最小的
	oldest := &cvl.Versions[0]
	for i := 1; i < len(cvl.Versions); i++ {
		if cvl.Versions[i].Version < oldest.Version {
			oldest = &cvl.Versions[i]
		}
	}
	return oldest
}

// GetByVersion 根据版本号获取检查点
func (cvl *CheckpointVersionList) GetByVersion(version int) *CheckpointVersion {
	for i := range cvl.Versions {
		if cvl.Versions[i].Version == version {
			return &cvl.Versions[i]
		}
	}
	return nil
}

// GetByID 根据 ID 获取检查点
func (cvl *CheckpointVersionList) GetByID(id string) *CheckpointVersion {
	for i := range cvl.Versions {
		if cvl.Versions[i].ID == id {
			return &cvl.Versions[i]
		}
	}
	return nil
}

// IsZero 检查 CleanupResult 是否为零值
func (cr *CleanupResult) IsZero() bool {
	return cr.DeletedCount == 0 && cr.FreedBytes == 0
}
