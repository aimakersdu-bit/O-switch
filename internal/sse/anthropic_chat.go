package sse

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"

	"baixin-switch/internal/openai"
)

type ChatStreamOptions struct {
	ResponseID string
	Model      string
}

func AnthropicToOpenAIChatStream(r io.Reader, opts ChatStreamOptions) (string, error) {
	var out strings.Builder
	if err := WriteAnthropicToOpenAIChatStream(&out, r, opts); err != nil {
		return "", err
	}
	return out.String(), nil
}

func WriteAnthropicToOpenAIChatStream(w io.Writer, r io.Reader, opts ChatStreamOptions) error {
	if opts.ResponseID == "" {
		opts.ResponseID = "chatcmpl_baixin"
	}

	writer := newSSEWriter(w)
	state := anthropicChatState{opts: opts}
	state.writeChunk(writer, openai.ChatCompletionChunk{
		ID:      opts.ResponseID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   opts.Model,
		Choices: []openai.Choice{{Index: 0, Delta: openai.Delta{Role: stringPtr("assistant")}}},
	})

	err := scanSSEBlocks(r, func(block string) error {
		_, data, _ := parseBlock(block)
		if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
			return nil
		}
		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil
		}
		state.handle(writer, event)
		return nil
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(writer, "data: [DONE]\n\n")
	writer.Flush()
	return err
}

type flushWriter interface {
	io.Writer
	Flush()
}

type sseWriter struct {
	io.Writer
	flusher interface{ Flush() }
}

func newSSEWriter(w io.Writer) flushWriter {
	out := &sseWriter{Writer: w}
	if f, ok := w.(interface{ Flush() }); ok {
		out.flusher = f
	}
	return out
}

func (w *sseWriter) Flush() {
	if w.flusher != nil {
		w.flusher.Flush()
	}
}

type anthropicChatState struct {
	opts       ChatStreamOptions
	stopReason string
}

func (s *anthropicChatState) handle(out flushWriter, event anthropicStreamEvent) {
	switch event.Type {
	case "message_start":
		if event.Message.Model != "" {
			s.opts.Model = event.Message.Model
		}
	case "content_block_start":
		if event.ContentBlock.Type == "tool_use" {
			empty := ""
			s.writeChunk(out, openai.ChatCompletionChunk{
				ID:      s.opts.ResponseID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   s.opts.Model,
				Choices: []openai.Choice{{
					Index: 0,
					Delta: openai.Delta{ToolCalls: []openai.ToolCall{{
						Index: event.Index,
						ID:    event.ContentBlock.ID,
						Type:  "function",
						Function: openai.ToolFunction{
							Name:      event.ContentBlock.Name,
							Arguments: &empty,
						},
					}}},
				}},
			})
		}
	case "content_block_delta":
		switch event.Delta.Type {
		case "text_delta":
			text := event.Delta.Text
			s.writeChunk(out, openai.ChatCompletionChunk{
				ID:      s.opts.ResponseID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   s.opts.Model,
				Choices: []openai.Choice{{Index: 0, Delta: openai.Delta{Content: &text}}},
			})
		case "input_json_delta":
			part := event.Delta.PartialJSON
			s.writeChunk(out, openai.ChatCompletionChunk{
				ID:      s.opts.ResponseID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   s.opts.Model,
				Choices: []openai.Choice{{
					Index: 0,
					Delta: openai.Delta{ToolCalls: []openai.ToolCall{{
						Index: event.Index,
						Function: openai.ToolFunction{
							Arguments: &part,
						},
					}}},
				}},
			})
		}
	case "message_delta":
		s.stopReason = mapAnthropicStopReason(event.Delta.StopReason)
		if s.stopReason != "" {
			s.writeChunk(out, openai.ChatCompletionChunk{
				ID:      s.opts.ResponseID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   s.opts.Model,
				Choices: []openai.Choice{{Index: 0, Delta: openai.Delta{}, FinishReason: &s.stopReason}},
			})
		}
	}
}

func (s *anthropicChatState) writeChunk(out flushWriter, chunk openai.ChatCompletionChunk) {
	raw, _ := json.Marshal(chunk)
	_, _ = io.WriteString(out, "data: ")
	_, _ = out.Write(raw)
	_, _ = io.WriteString(out, "\n\n")
	out.Flush()
}

type anthropicStreamEvent struct {
	Type         string                 `json:"type"`
	Index        int                    `json:"index"`
	Message      anthropicStreamMessage `json:"message"`
	ContentBlock anthropicContentBlock  `json:"content_block"`
	Delta        anthropicDelta         `json:"delta"`
}

type anthropicStreamMessage struct {
	ID    string `json:"id"`
	Model string `json:"model"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}

func mapAnthropicStopReason(reason string) string {
	switch reason {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "end_turn", "stop_sequence":
		return "stop"
	default:
		return ""
	}
}

func scanSSEBlocks(r io.Reader, handle func(string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var current strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if current.Len() > 0 {
				if err := handle(current.String()); err != nil {
					return err
				}
				current.Reset()
			}
			continue
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if current.Len() > 0 {
		return handle(current.String())
	}
	return nil
}
