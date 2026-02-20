package a2a

import "errors"

// 代理卡验证错误.
var (
	// 错失 名称表示代理卡缺少一个名称.
	ErrMissingName = errors.New("agent card: missing name")
	// ErrMissing Description 显示代理卡缺少描述 。
	ErrMissingDescription = errors.New("agent card: missing description")
	// ErrMissingURL 显示代理卡缺少一个 URL 。
	ErrMissingURL = errors.New("agent card: missing url")
	// ErrMissingVersion表示代理卡缺少一个版本.
	ErrMissingVersion = errors.New("agent card: missing version")
)

// A2A 协议错误 。
var (
	// ErrAgentNotFound表示未找到被请求的代理人.
	ErrAgentNotFound = errors.New("a2a: agent not found")
	// ErrRemote Uncomputing 表示远程代理无法使用 。
	ErrRemoteUnavailable = errors.New("a2a: remote agent unavailable")
	// ErrAuth 失败表示认证失败 。
	ErrAuthFailed = errors.New("a2a: authentication failed")
	// ErrInvalidMessage 表示信件格式无效 。
	ErrInvalidMessage = errors.New("a2a: invalid message format")
)

// A2A 信件验证错误 。
var (
	// ErrMessage MissingID 显示消息缺少一个ID.
	ErrMessageMissingID = errors.New("a2a message: missing id")
	// ErrMessage InvalidType 表示消息类型无效 。
	ErrMessageInvalidType = errors.New("a2a message: invalid type")
	// 误差 显示信件缺少发送者 。
	ErrMessageMissingFrom = errors.New("a2a message: missing from")
	// ErrMessage Missing To 表示信件缺少收件人 。
	ErrMessageMissingTo = errors.New("a2a message: missing to")
	// ErrMessage Missing Timestamp 显示消息缺少一个时间戳 。
	ErrMessageMissingTimestamp = errors.New("a2a message: missing timestamp")
)

// A2A客户端出错.
var (
	// ErrTask NotReady 表示正在处理同步任务 。
	ErrTaskNotReady = errors.New("a2a: task not ready")
	// ErrTaskNotFound 表示未找到任务 。
	ErrTaskNotFound = errors.New("a2a: task not found")
)
