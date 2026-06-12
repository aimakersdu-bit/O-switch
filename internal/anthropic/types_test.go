package anthropic_test

import (
	"encoding/json"
	"testing"

	"baixin-switch/internal/anthropic"
)

func TestMessageUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
		wantMsg anthropic.Message
	}{
		{
			name:    "content is plain string",
			jsonStr: `{"role": "user", "content": "hello world"}`,
			wantErr: false,
			wantMsg: anthropic.Message{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{
						Type: "text",
						Text: "hello world",
					},
				},
			},
		},
		{
			name:    "content is array of blocks",
			jsonStr: `{"role": "user", "content": [{"type": "text", "text": "hello block"}]}`,
			wantErr: false,
			wantMsg: anthropic.Message{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{
						Type: "text",
						Text: "hello block",
					},
				},
			},
		},
		{
			name:    "content has spaces before string",
			jsonStr: `{"role": "user", "content":   "hello spaces"}`,
			wantErr: false,
			wantMsg: anthropic.Message{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{
						Type: "text",
						Text: "hello spaces",
					},
				},
			},
		},
		{
			name:    "invalid content type",
			jsonStr: `{"role": "user", "content": 123}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg anthropic.Message
			err := json.Unmarshal([]byte(tt.jsonStr), &msg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if msg.Role != tt.wantMsg.Role {
				t.Errorf("Role = %q, want %q", msg.Role, tt.wantMsg.Role)
			}
			if len(msg.Content) != len(tt.wantMsg.Content) {
				t.Fatalf("Content len = %d, want %d", len(msg.Content), len(tt.wantMsg.Content))
			}
			for i, b := range msg.Content {
				wantB := tt.wantMsg.Content[i]
				if b.Type != wantB.Type || b.Text != wantB.Text {
					t.Errorf("Content[%d] = %+v, want %+v", i, b, wantB)
				}
			}
		})
	}
}
