package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		client := NewHTTPClient(nil)
		assert.NotNil(t, client)
		assert.Equal(t, 30*time.Second, client.config.Timeout)
		assert.Equal(t, 3, client.config.RetryCount)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &ClientConfig{
			Timeout:    10 * time.Second,
			RetryCount: 5,
			RetryDelay: 2 * time.Second,
			AgentID:    "test-agent",
		}
		client := NewHTTPClient(config)
		assert.NotNil(t, client)
		assert.Equal(t, 10*time.Second, client.config.Timeout)
		assert.Equal(t, 5, client.config.RetryCount)
	})
}

func TestHTTPClient_Discover(t *testing.T) {
	t.Run("successful discovery", func(t *testing.T) {
		card := &AgentCard{
			Name:        "test-agent",
			Description: "A test agent",
			URL:         "http://localhost:8080",
			Version:     "1.0.0",
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/.well-known/agent.json", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
		}))
		defer server.Close()

		client := NewHTTPClient(nil)
		result, err := client.Discover(context.Background(), server.URL)

		require.NoError(t, err)
		assert.Equal(t, card.Name, result.Name)
		assert.Equal(t, card.Description, result.Description)
		assert.Equal(t, card.Version, result.Version)
	})

	t.Run("empty url returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		_, err := client.Discover(context.Background(), "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrRemoteUnavailable)
	})

	t.Run("server returns 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		config := &ClientConfig{
			Timeout:    1 * time.Second,
			RetryCount: 0,
		}
		client := NewHTTPClient(config)
		_, err := client.Discover(context.Background(), server.URL)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrRemoteUnavailable)
	})

	t.Run("invalid json response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		client := NewHTTPClient(nil)
		_, err := client.Discover(context.Background(), server.URL)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("caches discovered card", func(t *testing.T) {
		callCount := 0
		card := &AgentCard{
			Name:        "test-agent",
			Description: "A test agent",
			URL:         "http://localhost:8080",
			Version:     "1.0.0",
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
		}))
		defer server.Close()

		client := NewHTTPClient(nil)

		// 第一通电话
		_, err := client.Discover(context.Background(), server.URL)
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)

		// 第二通电话应该使用缓存
		_, err = client.Discover(context.Background(), server.URL)
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
		}))
		defer server.Close()

		client := NewHTTPClient(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := client.Discover(ctx, server.URL)
		assert.Error(t, err)
	})
}

func TestHTTPClient_Send(t *testing.T) {
	t.Run("successful send", func(t *testing.T) {
		responseMsg := NewResultMessage("remote-agent", "local-agent", map[string]string{"result": "success"}, "msg-123")

		var serverURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/agent.json" {
				card := &AgentCard{
					Name:        "remote-agent",
					Description: "Remote agent",
					URL:         serverURL, // Use actual server URL
					Version:     "1.0.0",
				}
				json.NewEncoder(w).Encode(card)
				return
			}

			assert.Equal(t, "/a2a/messages", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var msg A2AMessage
			err := json.NewDecoder(r.Body).Decode(&msg)
			require.NoError(t, err)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(responseMsg)
		}))
		defer server.Close()
		serverURL = server.URL

		client := NewHTTPClient(nil)
		msg := NewTaskMessage("local-agent", server.URL, map[string]string{"task": "test"})

		result, err := client.Send(context.Background(), msg)

		require.NoError(t, err)
		assert.Equal(t, A2AMessageTypeResult, result.Type)
	})

	t.Run("nil message returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		_, err := client.Send(context.Background(), nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("invalid message returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		msg := &A2AMessage{} // Missing required fields
		_, err := client.Send(context.Background(), msg)
		assert.Error(t, err)
	})

	t.Run("server error", func(t *testing.T) {
		var serverURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/agent.json" {
				card := &AgentCard{
					Name:        "remote-agent",
					Description: "Remote agent",
					URL:         serverURL,
					Version:     "1.0.0",
				}
				json.NewEncoder(w).Encode(card)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()
		serverURL = server.URL

		config := &ClientConfig{
			Timeout:    1 * time.Second,
			RetryCount: 0,
		}
		client := NewHTTPClient(config)
		msg := NewTaskMessage("local-agent", server.URL, map[string]string{"task": "test"})

		_, err := client.Send(context.Background(), msg)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrRemoteUnavailable)
	})
}

func TestHTTPClient_SendAsync(t *testing.T) {
	t.Run("successful async send", func(t *testing.T) {
		var serverURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/agent.json" {
				card := &AgentCard{
					Name:        "remote-agent",
					Description: "Remote agent",
					URL:         serverURL,
					Version:     "1.0.0",
				}
				json.NewEncoder(w).Encode(card)
				return
			}

			assert.Equal(t, "/a2a/messages/async", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(AsyncResponse{
				TaskID: "task-123",
				Status: "accepted",
			})
		}))
		defer server.Close()
		serverURL = server.URL

		client := NewHTTPClient(nil)
		msg := NewTaskMessage("local-agent", server.URL, map[string]string{"task": "test"})

		taskID, err := client.SendAsync(context.Background(), msg)

		require.NoError(t, err)
		assert.Equal(t, "task-123", taskID)
	})

	t.Run("nil message returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		_, err := client.SendAsync(context.Background(), nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("missing task_id in response", func(t *testing.T) {
		var serverURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/agent.json" {
				card := &AgentCard{
					Name:        "remote-agent",
					Description: "Remote agent",
					URL:         serverURL,
					Version:     "1.0.0",
				}
				json.NewEncoder(w).Encode(card)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(AsyncResponse{
				Status: "accepted",
			})
		}))
		defer server.Close()
		serverURL = server.URL

		client := NewHTTPClient(nil)
		msg := NewTaskMessage("local-agent", server.URL, map[string]string{"task": "test"})

		_, err := client.SendAsync(context.Background(), msg)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})
}

func TestHTTPClient_GetResult(t *testing.T) {
	t.Run("empty task_id returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		_, err := client.GetResult(context.Background(), "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("unregistered task returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		_, err := client.GetResult(context.Background(), "unknown-task")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("successful get result with registered task", func(t *testing.T) {
		resultMsg := NewResultMessage("remote-agent", "local-agent", map[string]string{"result": "done"}, "task-123")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/a2a/tasks/task-123/result", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resultMsg)
		}))
		defer server.Close()

		client := NewHTTPClient(nil)
		// 手动注册任务
		client.RegisterTask("task-123", server.URL)

		result, err := client.GetResult(context.Background(), "task-123")
		require.NoError(t, err)
		assert.Equal(t, A2AMessageTypeResult, result.Type)
	})

	t.Run("get result after SendAsync", func(t *testing.T) {
		resultMsg := NewResultMessage("remote-agent", "local-agent", map[string]string{"result": "done"}, "task-456")

		var serverURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/.well-known/agent.json" {
				card := &AgentCard{
					Name:        "remote-agent",
					Description: "Remote agent",
					URL:         serverURL,
					Version:     "1.0.0",
				}
				json.NewEncoder(w).Encode(card)
				return
			}
			if r.URL.Path == "/a2a/messages/async" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(AsyncResponse{
					TaskID: "task-456",
					Status: "accepted",
				})
				return
			}
			if r.URL.Path == "/a2a/tasks/task-456/result" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resultMsg)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		serverURL = server.URL

		client := NewHTTPClient(nil)
		msg := NewTaskMessage("local-agent", server.URL, map[string]string{"task": "test"})

		// 发送同步
		taskID, err := client.SendAsync(context.Background(), msg)
		require.NoError(t, err)
		assert.Equal(t, "task-456", taskID)

		// 使用任务标识获取结果
		result, err := client.GetResult(context.Background(), taskID)
		require.NoError(t, err)
		assert.Equal(t, A2AMessageTypeResult, result.Type)
	})
}

func TestHTTPClient_GetResultFromAgent(t *testing.T) {
	t.Run("successful get result", func(t *testing.T) {
		resultMsg := NewResultMessage("remote-agent", "local-agent", map[string]string{"result": "done"}, "task-123")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/a2a/tasks/task-123/result", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resultMsg)
		}))
		defer server.Close()

		client := NewHTTPClient(nil)
		result, err := client.GetResultFromAgent(context.Background(), server.URL, "task-123")

		require.NoError(t, err)
		assert.Equal(t, A2AMessageTypeResult, result.Type)
	})

	t.Run("task not ready", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		client := NewHTTPClient(nil)
		_, err := client.GetResultFromAgent(context.Background(), server.URL, "task-123")

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrTaskNotReady)
	})

	t.Run("task not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewHTTPClient(nil)
		_, err := client.GetResultFromAgent(context.Background(), server.URL, "task-123")

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentNotFound)
	})

	t.Run("empty task_id returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		_, err := client.GetResultFromAgent(context.Background(), "http://localhost", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("empty agent_url returns error", func(t *testing.T) {
		client := NewHTTPClient(nil)
		_, err := client.GetResultFromAgent(context.Background(), "", "task-123")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrRemoteUnavailable)
	})
}

func TestHTTPClient_ClearCache(t *testing.T) {
	card := &AgentCard{
		Name:        "test-agent",
		Description: "A test agent",
		URL:         "http://localhost:8080",
		Version:     "1.0.0",
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card)
	}))
	defer server.Close()

	client := NewHTTPClient(nil)

	// 第一通电话
	_, err := client.Discover(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// 清除缓存
	client.ClearCache()

	// 第二通电话应该再打服务器
	_, err = client.Discover(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestHTTPClient_SetHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-value", r.Header.Get("X-Custom-Header"))
		card := &AgentCard{
			Name:        "test-agent",
			Description: "A test agent",
			URL:         "http://localhost:8080",
			Version:     "1.0.0",
		}
		json.NewEncoder(w).Encode(card)
	}))
	defer server.Close()

	client := NewHTTPClient(nil)
	client.SetHeader("X-Custom-Header", "test-value")

	_, err := client.Discover(context.Background(), server.URL)
	require.NoError(t, err)
}

func TestHTTPClient_SetTimeout(t *testing.T) {
	client := NewHTTPClient(nil)
	client.SetTimeout(5 * time.Second)

	assert.Equal(t, 5*time.Second, client.config.Timeout)
	assert.Equal(t, 5*time.Second, client.httpClient.Timeout)
}

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.RetryCount)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
	assert.Equal(t, "default-agent", config.AgentID)
	assert.NotNil(t, config.Headers)
}

func TestHTTPClient_TaskRegistry(t *testing.T) {
	t.Run("register and unregister task", func(t *testing.T) {
		client := NewHTTPClient(nil)

		// 注册任务
		client.RegisterTask("task-001", "http://agent.example.com")

		// 检查它注册( 通过检查 GetResult 不返回 ErrTask NotFound)
		_, err := client.GetResult(context.Background(), "task-001")
		// 连接出错时应当失败, 而不是 ErrTask NotFound
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrTaskNotFound)

		// 未注册任务
		client.UnregisterTask("task-001")

		// 现在它应该返回 ErrTask Not Found
		_, err = client.GetResult(context.Background(), "task-001")
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("clear task registry", func(t *testing.T) {
		client := NewHTTPClient(nil)

		// 注册多个任务
		client.RegisterTask("task-001", "http://agent1.example.com")
		client.RegisterTask("task-002", "http://agent2.example.com")

		// 清除登记册
		client.ClearTaskRegistry()

		// 两者应返回 ErrTask Not Found
		_, err := client.GetResult(context.Background(), "task-001")
		assert.ErrorIs(t, err, ErrTaskNotFound)

		_, err = client.GetResult(context.Background(), "task-002")
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("cleanup expired tasks", func(t *testing.T) {
		client := NewHTTPClient(nil)

		// 注册任务
		client.RegisterTask("task-old", "http://agent.example.com")

		// 手动设定创建时间为旧
		client.taskMu.Lock()
		client.taskRegistry["task-old"].createdAt = time.Now().Add(-2 * time.Hour)
		client.taskMu.Unlock()

		// 登记新任务
		client.RegisterTask("task-new", "http://agent.example.com")

		// 1小时以上的清理任务
		count := client.CleanupExpiredTasks(1 * time.Hour)
		assert.Equal(t, 1, count)

		// 旧任务应该走了
		_, err := client.GetResult(context.Background(), "task-old")
		assert.ErrorIs(t, err, ErrTaskNotFound)

		// 新任务应依然存在
		_, err = client.GetResult(context.Background(), "task-new")
		assert.NotErrorIs(t, err, ErrTaskNotFound)
	})
}
