package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Metrics struct {
	mu               sync.Mutex
	httpRequests     map[string]uint64
	conversionErrors map[string]uint64
	auditEvents      map[string]uint64
	activeRequests   atomic.Int64
	activeStreams    atomic.Int64
	auditQueueDepth  atomic.Int64
	inputTokens      atomic.Int64
	outputTokens     atomic.Int64
	totalTokens      atomic.Int64
}

func NewMetrics() *Metrics {
	return &Metrics{
		httpRequests:     map[string]uint64{},
		conversionErrors: map[string]uint64{},
		auditEvents:      map[string]uint64{},
	}
}

func (m *Metrics) IncHTTPRequests(method, path string, status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpRequests[fmt.Sprintf(`method="%s",path="%s",status="%d"`, escape(method), escape(path), status)]++
}

func (m *Metrics) IncConversionErrors(stage, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversionErrors[fmt.Sprintf(`stage="%s",reason="%s"`, escape(stage), escape(reason))]++
}

func (m *Metrics) IncAuditEvents(result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditEvents[fmt.Sprintf(`result="%s"`, escape(result))]++
}

func (m *Metrics) SetActiveRequests(v int64) {
	m.activeRequests.Store(v)
}

func (m *Metrics) SetActiveStreams(v int64) {
	m.activeStreams.Store(v)
}

func (m *Metrics) SetAuditQueueDepth(v int64) {
	m.auditQueueDepth.Store(v)
}

func (m *Metrics) AddTokens(input, output int) {
	if input < 0 {
		input = 0
	}
	if output < 0 {
		output = 0
	}
	m.inputTokens.Add(int64(input))
	m.outputTokens.Add(int64(output))
	m.totalTokens.Add(int64(input + output))
}

func (m *Metrics) Render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	writeCounterMap(&b, "baixin_http_requests_total", m.httpRequests)
	writeCounterMap(&b, "baixin_conversion_errors_total", m.conversionErrors)
	writeCounterMap(&b, "baixin_audit_events_total", m.auditEvents)
	fmt.Fprintf(&b, "baixin_active_requests %d\n", m.activeRequests.Load())
	fmt.Fprintf(&b, "baixin_active_streams %d\n", m.activeStreams.Load())
	fmt.Fprintf(&b, "baixin_audit_queue_depth %d\n", m.auditQueueDepth.Load())
	fmt.Fprintf(&b, "baixin_tokens_total{direction=\"input\"} %d\n", m.inputTokens.Load())
	fmt.Fprintf(&b, "baixin_tokens_total{direction=\"output\"} %d\n", m.outputTokens.Load())
	fmt.Fprintf(&b, "baixin_tokens_total{direction=\"total\"} %d\n", m.totalTokens.Load())
	return b.String()
}

func writeCounterMap(b *strings.Builder, name string, values map[string]uint64) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(b, "%s{%s} %d\n", name, key, values[key])
	}
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
