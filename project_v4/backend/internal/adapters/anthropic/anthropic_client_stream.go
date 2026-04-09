package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"keepstar_v4/internal/domain"
	"keepstar_v4/internal/ports"
)

// ============================================================================
// Streaming request/response types
// ============================================================================

// anthropicCachedStreamRequest extends anthropicCachedRequest with stream flag.
// Anthropic activates SSE when `stream:true` is present in the request body.
type anthropicCachedStreamRequest struct {
	Model      string                   `json:"model"`
	MaxTokens  int                      `json:"max_tokens"`
	System     []contentBlockWithCache  `json:"system"`
	Messages   []anthropicToolMsg       `json:"messages"`
	Tools      []anthropicToolWithCache `json:"tools,omitempty"`
	ToolChoice *toolChoiceConfig        `json:"tool_choice,omitempty"`
	Stream     bool                     `json:"stream"`
}

// sseEvent is a single parsed event from the Anthropic SSE stream.
// See https://docs.anthropic.com/en/api/messages-streaming
type sseEvent struct {
	Type string          // message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop, ping, error
	Data json.RawMessage // raw JSON payload of the event
}

// Parsed shapes for the specific event types we care about.

type sseMessageStart struct {
	Message struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			OutputTokens             int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type sseContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type  string                 `json:"type"` // "text" or "tool_use"
		ID    string                 `json:"id,omitempty"`
		Name  string                 `json:"name,omitempty"`
		Text  string                 `json:"text,omitempty"`
		Input map[string]interface{} `json:"input,omitempty"` // always {} at start for tool_use
	} `json:"content_block"`
}

type sseContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`                   // "text_delta" or "input_json_delta"
		Text        string `json:"text,omitempty"`         // for text_delta
		PartialJSON string `json:"partial_json,omitempty"` // for input_json_delta
	} `json:"delta"`
}

type sseContentBlockStop struct {
	Index int `json:"index"`
}

type sseMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason,omitempty"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type sseErrorEvent struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// ============================================================================
// StreamCallbacks — hook for real-time consumption.
// Phase 1 does not use these (collect-then-return). Phase 4 wires them to
// the StreamingToolDecoder + SSE handler.
// ============================================================================

// StreamCallbacks lets a caller observe the stream as it arrives.
// All fields are optional — nil callbacks are skipped. Returning a non-nil
// error from any callback aborts the stream and surfaces the error.
type StreamCallbacks struct {
	// OnToolUseStart fires when a tool_use content block begins. id/name are
	// known at this point, input is empty and will be filled by OnInputJSONDelta.
	OnToolUseStart func(blockIndex int, toolUseID, toolName string) error
	// OnInputJSONDelta fires for each partial_json chunk of a tool_use block.
	OnInputJSONDelta func(blockIndex int, partialJSON string) error
	// OnContentBlockStop fires when a content block is fully received.
	OnContentBlockStop func(blockIndex int) error
	// OnTextDelta fires for text deltas (not used for tool flow but kept for completeness).
	OnTextDelta func(blockIndex int, text string) error
}

// ============================================================================
// ChatWithToolsCachedStream — streaming variant of ChatWithToolsCached.
// Phase 1: collect-then-return. Uses SSE under the hood but produces the
// same LLMResponse shape as the non-streaming version. Existing callers can
// swap in this function with no semantic change, gaining only the ability
// to pass StreamCallbacks for observability.
// ============================================================================

func (c *Client) ChatWithToolsCachedStream(
	ctx context.Context,
	systemPrompt string,
	messages []domain.LLMMessage,
	tools []domain.ToolDefinition,
	cacheConfig *ports.CacheConfig,
	callbacks *StreamCallbacks,
) (*domain.LLMResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("anthropic API key not configured")
	}
	if cacheConfig == nil {
		return nil, fmt.Errorf("ChatWithToolsCachedStream requires cacheConfig (for parity with cached path)")
	}

	// ---- Build request body (mirrors ChatWithToolsCached) ----

	anthroTools := make([]anthropicToolWithCache, 0, len(tools))
	for i, t := range tools {
		tool := anthropicToolWithCache{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
		if cacheConfig.CacheTools && i == len(tools)-1 {
			tool.CacheControl = &cacheControl{Type: "ephemeral"}
		}
		anthroTools = append(anthroTools, tool)
	}

	systemBlocks := []contentBlockWithCache{
		{Type: "text", Text: systemPrompt},
	}
	if cacheConfig.CacheSystem {
		systemBlocks[0].CacheControl = &cacheControl{Type: "ephemeral"}
	}

	anthroMsgs := make([]anthropicToolMsg, 0, len(messages))
	for _, msg := range messages {
		anthroMsgs = append(anthroMsgs, convertToAnthropicMessage(msg))
	}
	if cacheConfig.CacheConversation && len(anthroMsgs) > 1 {
		lastHistoryIdx := len(anthroMsgs) - 2
		markMessageCacheControl(&anthroMsgs[lastHistoryIdx])
	}

	reqBody := anthropicCachedStreamRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemBlocks,
		Messages:  anthroMsgs,
		Tools:     anthroTools,
		Stream:    true,
	}
	if cacheConfig.ToolChoice != "" && cacheConfig.ToolChoice != "auto" {
		if cacheConfig.ToolChoice == "any" {
			reqBody.ToolChoice = &toolChoiceConfig{Type: "any"}
		} else if strings.HasPrefix(cacheConfig.ToolChoice, "tool:") {
			reqBody.ToolChoice = &toolChoiceConfig{
				Type: "tool",
				Name: strings.TrimPrefix(cacheConfig.ToolChoice, "tool:"),
			}
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "text/event-stream")

	// ---- Span instrumentation (mirrors ChatWithToolsCached) ----
	sc := domain.SpanFromContext(ctx)
	stage := domain.StageFromContext(ctx)
	reqSize := len(jsonBody)
	ttfbStart := time.Now()

	var endLLM func(...string)
	var endTTFB func(...string)
	if sc != nil && stage != "" {
		endLLM = sc.Start(stage + ".llm.stream")
		endTTFB = sc.Start(stage + ".llm.stream.ttfb")
		trace := &httptrace.ClientTrace{
			GotFirstResponseByte: func() {
				ttfbDuration := time.Since(ttfbStart)
				if ttfbDuration > 10*time.Second {
					c.log.Warn("slow LLM TTFB (stream)", "stage", stage, "duration", ttfbDuration)
				}
				if endTTFB != nil {
					endTTFB()
					endTTFB = nil
				}
			},
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if endTTFB != nil {
			endTTFB()
		}
		if endLLM != nil {
			endLLM()
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Non-2xx: read error body (may be plain JSON error, not SSE)
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		if endLLM != nil {
			endLLM()
		}
		return nil, fmt.Errorf("anthropic http %d: %s", resp.StatusCode, string(body))
	}

	// ---- Parse SSE stream and accumulate into an LLMResponse ----

	var endBody func(...string)
	if sc != nil && stage != "" {
		endBody = sc.Start(stage + ".llm.stream.body")
	}

	result, err := consumeSSEStream(resp.Body, callbacks)

	if endBody != nil {
		endBody()
	}
	respSize := 0
	if result != nil && result.Usage.OutputTokens > 0 {
		respSize = result.Usage.OutputTokens * 4
	}
	if endLLM != nil {
		detail := fmt.Sprintf("%dKB→%dKB", reqSize/1024, respSize/1024)
		endLLM(detail)
	}

	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, fmt.Errorf("empty stream result")
	}
	result.Usage.Model = c.model
	result.Usage.TotalTokens = result.Usage.InputTokens + result.Usage.OutputTokens +
		result.Usage.CacheCreationInputTokens + result.Usage.CacheReadInputTokens
	result.Usage.CostUSD = result.Usage.CalculateCost()
	return result, nil
}

// ============================================================================
// SSE parser — reads line-based SSE format and dispatches to accumulator.
// Anthropic SSE format:
//
//	event: <type>\n
//	data: <json>\n
//	\n
//
// Lines starting with ':' are comments (keepalives). Multi-line data is joined
// with '\n' per the SSE spec — in practice Anthropic keeps data to one line.
// ============================================================================

// streamAccumulator builds an LLMResponse from incoming SSE events and fires
// optional callbacks. It tracks content blocks by index: tool_use blocks
// accumulate partial_json into a buffer, text blocks accumulate text.
type streamAccumulator struct {
	result    *domain.LLMResponse
	blocks    map[int]*blockState // keyed by content_block index
	callbacks *StreamCallbacks
}

type blockState struct {
	kind      string // "text" or "tool_use"
	toolUseID string
	toolName  string
	textBuf   strings.Builder
	jsonBuf   strings.Builder
}

func newStreamAccumulator(cb *StreamCallbacks) *streamAccumulator {
	return &streamAccumulator{
		result:    &domain.LLMResponse{},
		blocks:    make(map[int]*blockState),
		callbacks: cb,
	}
}

func (a *streamAccumulator) handleEvent(ev sseEvent) error {
	switch ev.Type {
	case "message_start":
		var ms sseMessageStart
		if err := json.Unmarshal(ev.Data, &ms); err != nil {
			return fmt.Errorf("message_start decode: %w", err)
		}
		a.result.Usage.InputTokens = ms.Message.Usage.InputTokens
		a.result.Usage.OutputTokens = ms.Message.Usage.OutputTokens
		a.result.Usage.CacheCreationInputTokens = ms.Message.Usage.CacheCreationInputTokens
		a.result.Usage.CacheReadInputTokens = ms.Message.Usage.CacheReadInputTokens

	case "content_block_start":
		var cbs sseContentBlockStart
		if err := json.Unmarshal(ev.Data, &cbs); err != nil {
			return fmt.Errorf("content_block_start decode: %w", err)
		}
		bs := &blockState{kind: cbs.ContentBlock.Type}
		switch cbs.ContentBlock.Type {
		case "tool_use":
			bs.toolUseID = cbs.ContentBlock.ID
			bs.toolName = cbs.ContentBlock.Name
			if a.callbacks != nil && a.callbacks.OnToolUseStart != nil {
				if err := a.callbacks.OnToolUseStart(cbs.Index, bs.toolUseID, bs.toolName); err != nil {
					return err
				}
			}
		case "text":
			if cbs.ContentBlock.Text != "" {
				bs.textBuf.WriteString(cbs.ContentBlock.Text)
			}
		}
		a.blocks[cbs.Index] = bs

	case "content_block_delta":
		var cbd sseContentBlockDelta
		if err := json.Unmarshal(ev.Data, &cbd); err != nil {
			return fmt.Errorf("content_block_delta decode: %w", err)
		}
		bs, ok := a.blocks[cbd.Index]
		if !ok {
			return fmt.Errorf("delta for unknown block index %d", cbd.Index)
		}
		switch cbd.Delta.Type {
		case "text_delta":
			bs.textBuf.WriteString(cbd.Delta.Text)
			if a.callbacks != nil && a.callbacks.OnTextDelta != nil {
				if err := a.callbacks.OnTextDelta(cbd.Index, cbd.Delta.Text); err != nil {
					return err
				}
			}
		case "input_json_delta":
			bs.jsonBuf.WriteString(cbd.Delta.PartialJSON)
			if a.callbacks != nil && a.callbacks.OnInputJSONDelta != nil {
				if err := a.callbacks.OnInputJSONDelta(cbd.Index, cbd.Delta.PartialJSON); err != nil {
					return err
				}
			}
		}

	case "content_block_stop":
		var cbstop sseContentBlockStop
		if err := json.Unmarshal(ev.Data, &cbstop); err != nil {
			return fmt.Errorf("content_block_stop decode: %w", err)
		}
		bs, ok := a.blocks[cbstop.Index]
		if !ok {
			return fmt.Errorf("stop for unknown block index %d", cbstop.Index)
		}
		// Finalize block → append to result
		switch bs.kind {
		case "text":
			if a.result.Text == "" {
				a.result.Text = bs.textBuf.String()
			} else {
				a.result.Text += bs.textBuf.String()
			}
		case "tool_use":
			var input map[string]interface{}
			raw := bs.jsonBuf.String()
			if raw == "" {
				input = map[string]interface{}{}
			} else if err := json.Unmarshal([]byte(raw), &input); err != nil {
				return fmt.Errorf("tool_use input JSON decode (block %d): %w\nraw=%s", cbstop.Index, err, raw)
			}
			a.result.ToolCalls = append(a.result.ToolCalls, domain.ToolCall{
				ID:    bs.toolUseID,
				Name:  bs.toolName,
				Input: input,
			})
		}
		if a.callbacks != nil && a.callbacks.OnContentBlockStop != nil {
			if err := a.callbacks.OnContentBlockStop(cbstop.Index); err != nil {
				return err
			}
		}

	case "message_delta":
		var md sseMessageDelta
		if err := json.Unmarshal(ev.Data, &md); err != nil {
			return fmt.Errorf("message_delta decode: %w", err)
		}
		if md.Delta.StopReason != "" {
			a.result.StopReason = md.Delta.StopReason
		}
		if md.Usage.OutputTokens > 0 {
			a.result.Usage.OutputTokens = md.Usage.OutputTokens
		}

	case "message_stop":
		// No-op — terminal marker

	case "ping":
		// Keepalive — ignore

	case "error":
		var se sseErrorEvent
		if err := json.Unmarshal(ev.Data, &se); err != nil {
			return fmt.Errorf("error event decode: %w", err)
		}
		return fmt.Errorf("anthropic stream error (%s): %s", se.Error.Type, se.Error.Message)
	}
	return nil
}

// consumeSSEStream reads the SSE body line by line, parses events, and feeds
// them to the accumulator. Returns the assembled LLMResponse on successful
// terminal (message_stop), or an error if the stream is cut short or emits
// an error event.
func consumeSSEStream(body io.Reader, callbacks *StreamCallbacks) (*domain.LLMResponse, error) {
	reader := bufio.NewReader(body)
	acc := newStreamAccumulator(callbacks)

	var currentEvent string
	var dataBuf strings.Builder

	flush := func() error {
		if currentEvent == "" {
			// Event with no explicit type — per SSE spec defaults to "message",
			// but Anthropic always sets a type. Skip defensively.
			currentEvent = ""
			dataBuf.Reset()
			return nil
		}
		ev := sseEvent{Type: currentEvent, Data: json.RawMessage(dataBuf.String())}
		currentEvent = ""
		dataBuf.Reset()
		return acc.handleEvent(ev)
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			// Strip trailing \r\n or \n
			line = strings.TrimRight(line, "\r\n")

			switch {
			case line == "":
				// End of event — flush
				if fErr := flush(); fErr != nil {
					return acc.result, fErr
				}
			case strings.HasPrefix(line, ":"):
				// Comment / keepalive — ignore
			case strings.HasPrefix(line, "event:"):
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				// SSE allows multi-line data — join with \n per spec
				payload := strings.TrimPrefix(line, "data:")
				if strings.HasPrefix(payload, " ") {
					payload = payload[1:]
				}
				if dataBuf.Len() > 0 {
					dataBuf.WriteByte('\n')
				}
				dataBuf.WriteString(payload)
			default:
				// Unknown field — ignore per SSE spec
			}
		}

		if err == io.EOF {
			// Flush any pending event (shouldn't happen with Anthropic, but be safe)
			if dataBuf.Len() > 0 || currentEvent != "" {
				_ = flush()
			}
			return acc.result, nil
		}
		if err != nil {
			return acc.result, fmt.Errorf("read SSE line: %w", err)
		}
	}
}
