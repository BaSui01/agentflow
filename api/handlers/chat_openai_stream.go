package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
)

type openAICompatResponsesStreamEvent struct {
	name    string
	payload any
}

func (h *ChatHandler) handleOpenAICompatChatCompletionsStream(w http.ResponseWriter, r *http.Request, req *api.ChatRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAICompatError(w, types.NewInternalError("streaming not supported"))
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
		return
	}
	stream, err := service.Stream(r.Context(), h.converter.ToUsecaseRequest(req))
	if err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	created := time.Now().Unix()
	model := req.Model
	for chunk := range stream {
		if chunk.Err != nil {
			_ = writeSSEJSON(w, openAICompatErrorEnvelope{
				Error: openAICompatError{
					Message: chunk.Err.Message,
					Type:    "server_error",
					Code:    string(chunk.Err.Code),
				},
			})
			_ = writeSSE(w, []byte("data: [DONE]\n\n"))
			flusher.Flush()
			return
		}
		if chunk.Chunk == nil {
			continue
		}
		if strings.TrimSpace(chunk.Chunk.Model) != "" {
			model = chunk.Chunk.Model
		}
		payload := toOpenAICompatChatChunkResponse(chunk.Chunk, created, model)
		if err := writeSSEJSON(w, payload); err != nil {
			return
		}
		flusher.Flush()
	}

	_ = writeSSE(w, []byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func (h *ChatHandler) handleOpenAICompatResponsesStream(w http.ResponseWriter, r *http.Request, req *api.ChatRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAICompatError(w, types.NewInternalError("streaming not supported"))
		return
	}

	service, svcErr := h.currentServiceOrUnavailable("chat")
	if svcErr != nil {
		writeOpenAICompatError(w, svcErr)
		return
	}
	stream, err := service.Stream(r.Context(), h.converter.ToUsecaseRequest(req))
	if err != nil {
		writeOpenAICompatError(w, err)
		return
	}

	responseID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	model := req.Model
	createdEvent := map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":    responseID,
			"model": model,
		},
	}
	_ = writeSSEEventJSON(w, "response.created", createdEvent)
	flusher.Flush()

	for chunk := range stream {
		if chunk.Err != nil {
			_ = writeSSEEventJSON(w, "error", map[string]any{
				"type":    "error",
				"code":    string(chunk.Err.Code),
				"message": chunk.Err.Message,
			})
			_ = writeSSE(w, []byte("data: [DONE]\n\n"))
			flusher.Flush()
			return
		}
		if chunk.Chunk == nil {
			continue
		}
		if strings.TrimSpace(chunk.Chunk.Model) != "" {
			model = chunk.Chunk.Model
		}
		for _, ev := range toOpenAICompatResponsesStreamEvents(chunk.Chunk) {
			if err := writeSSEEventJSON(w, ev.name, ev.payload); err != nil {
				return
			}
		}
		flusher.Flush()
	}

	completedEvent := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":    responseID,
			"model": model,
		},
	}
	_ = writeSSEEventJSON(w, "response.completed", completedEvent)
	_ = writeSSE(w, []byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func toOpenAICompatResponsesStreamEvents(chunk *usecase.ChatStreamChunk) []openAICompatResponsesStreamEvent {
	if chunk == nil {
		return nil
	}
	events := make([]openAICompatResponsesStreamEvent, 0, 4)
	content := strings.TrimSpace(chunk.Delta.Content)
	if content != "" {
		events = append(events, openAICompatResponsesStreamEvent{
			name: "response.output_text.delta",
			payload: map[string]any{
				"type":  "response.output_text.delta",
				"delta": content,
			},
		})
	}
	for _, call := range chunk.Delta.ToolCalls {
		itemID := strings.TrimSpace(call.ID)
		if itemID == "" {
			itemID = fmt.Sprintf("fc_%d", time.Now().UnixNano())
		}
		callType := strings.TrimSpace(call.Type)
		if callType == "" {
			callType = types.ToolTypeFunction
		}
		switch callType {
		case types.ToolTypeCustom:
			events = append(events, openAICompatResponsesStreamEvent{
				name: "response.custom_tool_call_input.delta",
				payload: map[string]any{
					"type":    "response.custom_tool_call_input.delta",
					"item_id": itemID,
					"name":    call.Name,
					"delta":   call.Input,
				},
			})
			events = append(events, openAICompatResponsesStreamEvent{
				name: "response.custom_tool_call_input.done",
				payload: map[string]any{
					"type":    "response.custom_tool_call_input.done",
					"item_id": itemID,
				},
			})
		default:
			events = append(events, openAICompatResponsesStreamEvent{
				name: "response.function_call_arguments.delta",
				payload: map[string]any{
					"type":    "response.function_call_arguments.delta",
					"item_id": itemID,
					"name":    call.Name,
					"delta":   string(call.Arguments),
				},
			})
			events = append(events, openAICompatResponsesStreamEvent{
				name: "response.function_call_arguments.done",
				payload: map[string]any{
					"type":    "response.function_call_arguments.done",
					"item_id": itemID,
				},
			})
		}
	}
	if chunk.FinishReason == "stop" {
		events = append(events, openAICompatResponsesStreamEvent{
			name: "response.output_text.done",
			payload: map[string]any{
				"type": "response.output_text.done",
			},
		})
	}
	return events
}

func writeSSEJSON(w http.ResponseWriter, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSE(w, []byte("data: "), data, []byte("\n\n"))
}

func writeSSEEventJSON(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSE(w, []byte("event: "+event+"\n"), []byte("data: "), data, []byte("\n\n"))
}
