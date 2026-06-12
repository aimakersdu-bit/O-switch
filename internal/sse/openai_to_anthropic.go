package sse

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	"baixin-switch/internal/openai"
)

type OpenAIStreamOptions struct {
	MessageID   string
	Model       string
	OnUsage     func(Usage)
	OnTextDelta func(string)
}

func WriteOpenAIToAnthropicStream(w io.Writer, r io.Reader, opts OpenAIStreamOptions) error {
	if opts.MessageID == "" {
		opts.MessageID = generateMsgID()
	}

	writer := newSSEWriter(w)
	state := openAIChatToAnthropicState{
		opts:      opts,
		messageID: opts.MessageID,
		model:     opts.Model,
	}

	// 1. Emit message_start event
	messageStartData := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            state.messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         state.model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	state.emitEvent(writer, "message_start", messageStartData)

	err := scanSSEBlocks(r, func(block string) error {
		_, data, _ := parseBlock(block)
		if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
			return nil
		}
		var chunk openai.ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil
		}
		state.handle(writer, chunk)
		return nil
	})
	if err != nil {
		return err
	}

	// Ensure active content block stops if any
	state.stopActiveBlock(writer)

	// 2. Emit message_delta
	stopReason := "end_turn"
	if state.stopReason != "" {
		stopReason = state.stopReason
	}
	messageDeltaData := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": state.outputTokens,
		},
	}
	state.emitEvent(writer, "message_delta", messageDeltaData)

	// 3. Emit message_stop
	messageStopData := map[string]any{
		"type": "message_stop",
	}
	state.emitEvent(writer, "message_stop", messageStopData)

	if opts.OnUsage != nil {
		opts.OnUsage(Usage{
			InputTokens:  state.inputTokens,
			OutputTokens: state.outputTokens,
		})
	}

	return nil
}

type openAIChatToAnthropicState struct {
	opts              OpenAIStreamOptions
	messageID         string
	model             string
	contentBlockIndex int
	textBlockStarted  bool
	activeToolCallID  string
	stopReason        string
	inputTokens       int
	outputTokens      int
}

func (s *openAIChatToAnthropicState) handle(out flushWriter, chunk openai.ChatCompletionChunk) {
	if len(chunk.Choices) == 0 {
		// Even if choices is empty, there might be usage details in the chunk.
		// Let's not return early if usage is present.
		if chunk.Usage == nil {
			return
		}
	}
	choice := openai.Choice{}
	if len(chunk.Choices) > 0 {
		choice = chunk.Choices[0]
	}

	// Handle usage if present
	if chunk.Usage != nil {
		if raw, err := json.Marshal(chunk.Usage); err == nil {
			var u struct {
				PromptTokens     int `json:"prompt_tokens"`
				InputTokens      int `json:"input_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				OutputTokens     int `json:"output_tokens"`
			}
			if json.Unmarshal(raw, &u) == nil {
				if u.PromptTokens > 0 {
					s.inputTokens = u.PromptTokens
				} else if u.InputTokens > 0 {
					s.inputTokens = u.InputTokens
				}
				if u.CompletionTokens > 0 {
					s.outputTokens = u.CompletionTokens
				} else if u.OutputTokens > 0 {
					s.outputTokens = u.OutputTokens
				}
				if s.opts.OnUsage != nil {
					s.opts.OnUsage(Usage{
						InputTokens:  s.inputTokens,
						OutputTokens: s.outputTokens,
					})
				}
			}
		}
	}

	// 1. Text content delta
	if choice.Delta.Content != nil && *choice.Delta.Content != "" {
		text := *choice.Delta.Content
		if !s.textBlockStarted {
			// Stop active tool block if any before starting text
			s.stopActiveBlock(out)
			// Emit content_block_start for text
			startData := map[string]any{
				"type":  "content_block_start",
				"index": s.contentBlockIndex,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			}
			s.emitEvent(out, "content_block_start", startData)
			s.textBlockStarted = true
		}
		// Emit content_block_delta
		deltaData := map[string]any{
			"type":  "content_block_delta",
			"index": s.contentBlockIndex,
			"delta": map[string]any{
				"type": "text_delta",
				"text": text,
			},
		}
		s.emitEvent(out, "content_block_delta", deltaData)
		if s.opts.OnTextDelta != nil {
			s.opts.OnTextDelta(text)
		}
	}

	// 2. Tool calls delta
	for _, tc := range choice.Delta.ToolCalls {
		if tc.ID != "" && tc.ID != s.activeToolCallID {
			// Stop active text block or previous tool block
			s.stopActiveBlock(out)
			// Start new tool_use block
			startData := map[string]any{
				"type":  "content_block_start",
				"index": s.contentBlockIndex,
				"content_block": map[string]any{
					"type": "tool_use",
					"id":   tc.ID,
					"name": tc.Function.Name,
					"input": map[string]any{},
				},
			}
			s.emitEvent(out, "content_block_start", startData)
			s.activeToolCallID = tc.ID
		}
		if tc.Function.Arguments != nil && *tc.Function.Arguments != "" {
			deltaData := map[string]any{
				"type":  "content_block_delta",
				"index": s.contentBlockIndex,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": *tc.Function.Arguments,
				},
			}
			s.emitEvent(out, "content_block_delta", deltaData)
		}
	}

	if choice.FinishReason != nil {
		s.stopReason = mapOpenAIFinishReasonToAnthropic(*choice.FinishReason)
	}
}

func (s *openAIChatToAnthropicState) stopActiveBlock(out flushWriter) {
	if s.textBlockStarted || s.activeToolCallID != "" {
		stopData := map[string]any{
			"type":  "content_block_stop",
			"index": s.contentBlockIndex,
		}
		s.emitEvent(out, "content_block_stop", stopData)
		s.contentBlockIndex++
		s.textBlockStarted = false
		s.activeToolCallID = ""
	}
}

func (s *openAIChatToAnthropicState) emitEvent(out flushWriter, eventType string, data any) {
	raw, _ := json.Marshal(data)
	_, _ = io.WriteString(out, "event: "+eventType+"\n")
	_, _ = io.WriteString(out, "data: "+string(raw)+"\n\n")
	out.Flush()
}

func mapOpenAIFinishReasonToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}

func generateMsgID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "msg_" + hex.EncodeToString(b[:])
}
