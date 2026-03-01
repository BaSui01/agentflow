package agent

import (
	"github.com/BaSui01/agentflow/agent/memorycore"
)

// MemoryKind 记忆类型。
type MemoryKind = memorycore.MemoryKind

const (
	MemoryShortTerm  MemoryKind = memorycore.MemoryShortTerm
	MemoryWorking    MemoryKind = memorycore.MemoryWorking
	MemoryLongTerm   MemoryKind = memorycore.MemoryLongTerm
	MemoryEpisodic   MemoryKind = memorycore.MemoryEpisodic
	MemorySemantic   MemoryKind = memorycore.MemorySemantic
	MemoryProcedural MemoryKind = memorycore.MemoryProcedural
)

// MemoryRecord 统一记忆结构。
type MemoryRecord = memorycore.MemoryRecord

// MemoryWriter 记忆写入接口
type MemoryWriter = memorycore.MemoryWriter

// MemoryReader 记忆读取接口
type MemoryReader = memorycore.MemoryReader

// MemoryManager 组合读写接口
type MemoryManager = memorycore.MemoryManager

// keep root-level constant used by existing tests and base agent cache logic.
const defaultMaxRecentMemory = memorycore.MaxRecentMemory

