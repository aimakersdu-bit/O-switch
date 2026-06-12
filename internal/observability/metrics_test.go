package observability

import (
	"strings"
	"testing"
)

func TestMetricsRenderPrometheusText(t *testing.T) {
	m := NewMetrics()
	m.IncHTTPRequests("POST", "/v1/chat/completions", 200)
	m.IncConversionErrors("openai_chat", "bad_tool")
	m.SetActiveRequests(3)

	out := m.Render()
	for _, want := range []string{
		`baixin_http_requests_total{method="POST",path="/v1/chat/completions",status="200"} 1`,
		`baixin_conversion_errors_total{stage="openai_chat",reason="bad_tool"} 1`,
		`baixin_active_requests 3`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("metrics missing %q in:\n%s", want, out)
		}
	}
}
