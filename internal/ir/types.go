package ir

type Request struct {
	SourceAPI     string
	Model         string
	System        []string
	Messages      []Message
	Tools         []Tool
	ToolChoice    ToolChoice
	MaxTokens     int
	Temperature   *float64
	TopP          *float64
	StopSequences []string
	Stream        bool
}

type Message struct {
	Role    string
	Content []ContentBlock
}

type ContentBlock struct {
	Type       string
	Text       string
	ToolUse    *ToolUse
	ToolResult *ToolResult
}

type ToolUse struct {
	ID    string
	Name  string
	Input any
}

type ToolResult struct {
	ToolUseID string
	Content   string
}

type Tool struct {
	Name        string
	Description string
	InputSchema any
}

type ToolChoice struct {
	Type string
	Name string
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}
