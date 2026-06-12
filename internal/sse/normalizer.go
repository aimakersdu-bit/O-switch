package sse

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"baixin-switch/internal/openai"
)

type Options struct {
	ToolCallStreamShim bool
	ArgumentChunkSize  int
}

func NormalizeChatCompletionStream(r io.Reader, opts Options) (string, error) {
	if opts.ArgumentChunkSize <= 0 {
		opts.ArgumentChunkSize = 16
	}

	blocks, err := readSSEBlocks(r)
	if err != nil {
		return "", err
	}

	state := normalizerState{
		opts:          opts,
		seenToolMeta: map[string]bool{},
	}
	var out strings.Builder
	for _, block := range blocks {
		normalized, err := state.normalizeBlock(block)
		if err != nil {
			return "", err
		}
		for _, event := range normalized {
			out.WriteString(event)
		}
	}
	return out.String(), nil
}

type normalizerState struct {
	opts          Options
	seenToolMeta map[string]bool
}

func (s *normalizerState) normalizeBlock(block string) ([]string, error) {
	eventName, data, passthrough := parseBlock(block)
	if strings.TrimSpace(data) == "[DONE]" {
		return []string{formatBlock(eventName, "[DONE]")}, nil
	}
	if strings.TrimSpace(data) == "" {
		return []string{passthrough}, nil
	}

	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return []string{passthrough}, nil
	}
	if !s.opts.ToolCallStreamShim {
		return []string{passthrough}, nil
	}

	replacements := s.splitOneShotToolCalls(chunk)
	if len(replacements) == 0 {
		return []string{passthrough}, nil
	}

	out := make([]string, 0, len(replacements))
	for _, replacement := range replacements {
		raw, err := json.Marshal(replacement)
		if err != nil {
			return nil, err
		}
		out = append(out, formatBlock(eventName, string(raw)))
	}
	return out, nil
}

func (s *normalizerState) splitOneShotToolCalls(chunk openai.ChatCompletionChunk) []openai.ChatCompletionChunk {
	if len(chunk.Choices) != 1 {
		return nil
	}
	choice := chunk.Choices[0]
	if len(choice.Delta.ToolCalls) != 1 {
		return nil
	}
	call := choice.Delta.ToolCalls[0]
	if call.Function.Arguments == nil || *call.Function.Arguments == "" {
		return nil
	}

	key := toolKey(choice.Index, call.Index)
	hasMeta := call.ID != "" || call.Type != "" || call.Function.Name != ""
	if !hasMeta || s.seenToolMeta[key] {
		return nil
	}

	argParts := splitRunes(*call.Function.Arguments, s.opts.ArgumentChunkSize)
	if len(argParts) <= 1 {
		return nil
	}

	s.seenToolMeta[key] = true
	out := make([]openai.ChatCompletionChunk, 0, len(argParts)+1)

	meta := openai.CloneChunk(chunk)
	empty := ""
	meta.Choices[0].Delta.ToolCalls[0].Function.Arguments = &empty
	out = append(out, meta)

	for _, part := range argParts {
		next := openai.CloneChunk(chunk)
		next.Choices[0].Delta.Role = nil
		next.Choices[0].Delta.Content = nil
		next.Choices[0].Delta.ReasoningContent = nil
		next.Choices[0].Delta.ToolCalls = []openai.ToolCall{{
			Index: call.Index,
			Function: openai.ToolFunction{
				Arguments: stringPtr(part),
			},
		}}
		out = append(out, next)
	}

	return out
}

func stringPtr(s string) *string {
	return &s
}

func readSSEBlocks(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var blocks []string
	var current bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if current.Len() > 0 {
				blocks = append(blocks, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks, nil
}

func parseBlock(block string) (eventName string, data string, passthrough string) {
	var dataLines []string
	for _, line := range strings.Split(strings.TrimSuffix(block, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return eventName, strings.Join(dataLines, "\n"), ensureBlockSuffix(block)
}

func formatBlock(eventName, data string) string {
	var b strings.Builder
	if eventName != "" {
		b.WriteString("event: ")
		b.WriteString(eventName)
		b.WriteString("\n")
	}
	b.WriteString("data: ")
	b.WriteString(data)
	b.WriteString("\n\n")
	return b.String()
}

func ensureBlockSuffix(block string) string {
	block = strings.TrimRight(block, "\n")
	return block + "\n\n"
}

func toolKey(choiceIndex, toolIndex int) string {
	return fmt.Sprintf("%d:%d", choiceIndex, toolIndex)
}

func splitRunes(s string, size int) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}
	var parts []string
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		parts = append(parts, string(runes[start:end]))
	}
	return parts
}
