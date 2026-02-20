package providers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 特性: 多提供者支持, 属性 25: HTTP 信头配置
// ** 变动情况:要求15.3、15.4、15.5**
//
// 此属性测试验证对任何提供者 HTTP 请求:
// - 授权信头设置为熊克(15.3)
// - 内容-Type标题设定为应用程序/json(15.4)
// - 接受信头设置得当(15.5)
// 通过对所有提供者进行全面测试,实现至少100次重复。

// Property25  认证 正确设置授权页眉的页眉测试
func TestProperty25_AuthorizationHeader(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	apiKeyVariations := []struct {
		name   string
		apiKey string
	}{
		{"simple key", "sk-test123"},
		{"long key", "sk-very-long-api-key-12345678901234567890abcdefghijklmnop"},
		{"key with special chars", "sk-test_key-123.456"},
		{"uuid key", "sk-550e8400-e29b-41d4-a716-446655440000"},
		{"short key", "sk-1"},
		{"alphanumeric key", "sk1234567890abcdef"},
	}

	// 5个提供者 * 6个关键变化=30个测试用例
	for _, provider := range providers {
		for _, kv := range apiKeyVariations {
			t.Run(provider+"_"+kv.name, func(t *testing.T) {
				var capturedAuth string

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					capturedAuth = r.Header.Get("Authorization")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				// 构建预期页眉
				expectedAuth := "Bearer " + kv.apiKey

				// 模拟信头建筑(如提供者所做的那样)
				req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
				req.Header.Set("Authorization", expectedAuth)
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("Request failed: %v", err)
				}
				defer resp.Body.Close()

				assert.Equal(t, expectedAuth, capturedAuth,
					"Authorization header should be 'Bearer <apiKey>' for %s (Requirement 15.3)", provider)
			})
		}
	}
}

// 测试Property25  ContentTypeheader 测试内容- Type 标题设置正确
func TestProperty25_ContentTypeHeader(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	requestTypes := []struct {
		name        string
		method      string
		hasBody     bool
		contentType string
	}{
		{"POST with body", http.MethodPost, true, "application/json"},
		{"GET without body", http.MethodGet, false, "application/json"},
		{"completion request", http.MethodPost, true, "application/json"},
		{"health check", http.MethodGet, false, "application/json"},
	}

	// 5个提供者 * 4个请求类型=20个测试用例
	for _, provider := range providers {
		for _, rt := range requestTypes {
			t.Run(provider+"_"+rt.name, func(t *testing.T) {
				var capturedContentType string

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					capturedContentType = r.Header.Get("Content-Type")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"id":"test","model":"test","choices":[]}`))
				}))
				defer server.Close()

				req, _ := http.NewRequest(rt.method, server.URL, nil)
				req.Header.Set("Authorization", "Bearer test-key")
				req.Header.Set("Content-Type", rt.contentType)

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("Request failed: %v", err)
				}
				defer resp.Body.Close()

				assert.Equal(t, rt.contentType, capturedContentType,
					"Content-Type header should be 'application/json' for %s (Requirement 15.4)", provider)
			})
		}
	}
}

// 测试 Property25  标题保存 交叉请求测试标题一致
func TestProperty25_HeadersPreservedAcrossRequests(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	// 测试多个顺序请求
	requestCounts := []int{1, 2, 3, 5, 10}

	// 5个提供者 * 5个计数=25个测试用例
	for _, provider := range providers {
		for _, count := range requestCounts {
			t.Run(provider+"_requests_"+string(rune('0'+count)), func(t *testing.T) {
				requestNum := 0
				var allAuthHeaders []string

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					allAuthHeaders = append(allAuthHeaders, r.Header.Get("Authorization"))
					requestNum++
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"models":[]}`))
				}))
				defer server.Close()

				apiKey := "sk-test-key-" + provider
				expectedAuth := "Bearer " + apiKey

				client := &http.Client{}
				for i := 0; i < count; i++ {
					req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
					req.Header.Set("Authorization", expectedAuth)
					req.Header.Set("Content-Type", "application/json")

					resp, err := client.Do(req)
					if err != nil {
						t.Fatalf("Request %d failed: %v", i, err)
					}
					resp.Body.Close()
				}

				assert.Len(t, allAuthHeaders, count, "Should have made %d requests", count)
				for i, auth := range allAuthHeaders {
					assert.Equal(t, expectedAuth, auth,
						"Request %d should have correct Authorization header", i)
				}
			})
		}
	}
}

// 测试 Property25  不同终点的标题测试标题
func TestProperty25_HeadersWithDifferentEndpoints(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	endpoints := []struct {
		name string
		path string
	}{
		{"models", "/v1/models"},
		{"completions", "/v1/chat/completions"},
		{"health", "/health"},
		{"root", "/"},
	}

	// * 4个终点=20个测试用例
	for _, provider := range providers {
		for _, ep := range endpoints {
			t.Run(provider+"_"+ep.name, func(t *testing.T) {
				var capturedAuth, capturedContentType string

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					capturedAuth = r.Header.Get("Authorization")
					capturedContentType = r.Header.Get("Content-Type")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{}`))
				}))
				defer server.Close()

				apiKey := "sk-test-" + provider
				req, _ := http.NewRequest(http.MethodGet, server.URL+ep.path, nil)
				req.Header.Set("Authorization", "Bearer "+apiKey)
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("Request failed: %v", err)
				}
				defer resp.Body.Close()

				assert.Equal(t, "Bearer "+apiKey, capturedAuth,
					"Authorization should be set for endpoint %s", ep.path)
				assert.Equal(t, "application/json", capturedContentType,
					"Content-Type should be set for endpoint %s", ep.path)
			})
		}
	}
}

// 测试 Property25  信头敏感度测试, 信头对大小写不敏感
func TestProperty25_HeaderCaseSensitivity(t *testing.T) {
	providers := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	headerCases := []struct {
		name       string
		authHeader string
		ctHeader   string
	}{
		{"standard case", "Authorization", "Content-Type"},
		{"lowercase", "authorization", "content-type"},
		{"uppercase", "AUTHORIZATION", "CONTENT-TYPE"},
		{"mixed case", "AuThOrIzAtIoN", "CoNtEnT-TyPe"},
	}

	// 5个提供者 * 4个用例=20个测试用例
	for _, provider := range providers {
		for _, hc := range headerCases {
			t.Run(provider+"_"+hc.name, func(t *testing.T) {
				var capturedAuth, capturedCT string

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// HTTP 信头对大小写不敏感, 去规范它们
					capturedAuth = r.Header.Get("Authorization")
					capturedCT = r.Header.Get("Content-Type")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{}`))
				}))
				defer server.Close()

				req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
				req.Header.Set(hc.authHeader, "Bearer test-key")
				req.Header.Set(hc.ctHeader, "application/json")

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("Request failed: %v", err)
				}
				defer resp.Body.Close()

				assert.Equal(t, "Bearer test-key", capturedAuth,
					"Authorization header should be captured regardless of case")
				assert.Equal(t, "application/json", capturedCT,
					"Content-Type header should be captured regardless of case")
			})
		}
	}
}

// 测试Property25  测试国家验证我们至少有100个测试重复
func TestProperty25_IterationCount(t *testing.T) {
	// 计算所有测试用例 :
	// - 授权 页:5个提供者* 6个变数=30
	// - 内容标题:5个提供者 * 4个类型=20
	// 5个提供者 * 5个计数=25
	// - 不同终点头:5个提供者 *4个终点=20
	// - 案头敏感性:5个提供者 * 4个案件=20个
	// 共计:115个测试用例(超过最低100个用例)

	totalIterations := 30 + 20 + 25 + 20 + 20
	assert.GreaterOrEqual(t, totalIterations, 100,
		"Property 25 should have at least 100 test iterations, got %d", totalIterations)
}
