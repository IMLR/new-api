package cline

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// errorBodyLimit caps how much of a failed upstream response is buffered while
// looking for a daily cap window. Real Cline errors are a few hundred bytes.
const errorBodyLimit = 1 << 20

// UpstreamError represents an error frame that Cline emits inside an HTTP 200
// SSE stream. The original message is kept so clients see the real cause
// instead of a generic relay failure.
type UpstreamError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *UpstreamError) Error() string { return e.Message }

// upstreamErrorStatus maps a Cline error frame onto the HTTP status New API
// should report, which also drives channel retry and disable decisions.
func upstreamErrorStatus(code, message string) int {
	lower := strings.ToLower(code + " " + message)
	switch {
	case strings.Contains(lower, "429"),
		strings.Contains(lower, "rate limit"),
		strings.Contains(lower, "cap_error"),
		strings.Contains(lower, "quota"),
		strings.Contains(lower, "daily free limit"),
		strings.Contains(lower, "daily limit"):
		return http.StatusTooManyRequests
	case strings.Contains(lower, "401"),
		strings.Contains(lower, "403"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "invalid token"),
		strings.Contains(lower, "revoked"):
		return http.StatusUnauthorized
	}
	return http.StatusBadGateway
}

func parseUpstreamError(raw any) *UpstreamError {
	code, message := "", ""
	switch value := raw.(type) {
	case string:
		message = strings.TrimSpace(value)
	case map[string]any:
		if text, ok := value["message"].(string); ok {
			message = strings.TrimSpace(text)
		}
		for _, key := range []string{"code", "type"} {
			if text, ok := value[key].(string); ok && text != "" {
				code = text
				break
			}
		}
		if message == "" {
			if encoded, err := common.Marshal(value); err == nil {
				message = string(encoded)
			}
		}
	}
	if message == "" {
		message = "Cline upstream returned an empty error"
	}
	return &UpstreamError{StatusCode: upstreamErrorStatus(code, message), Code: code, Message: message}
}

// BuildErrorBody renders the upstream error as an OpenAI-compatible body so the
// standard relay error path can parse it.
func (e *UpstreamError) BuildErrorBody() []byte {
	body, err := common.Marshal(map[string]any{
		"error": map[string]any{
			"message": e.Message,
			"type":    "cline_upstream_error",
			"code":    e.Code,
		},
	})
	if err != nil {
		return []byte(`{"error":{"message":"Cline upstream stream failed","type":"cline_upstream_error"}}`)
	}
	return body
}

// parseErrorBody reads the error object out of an upstream HTTP failure body.
// Cline has served the same quota error both as an SSE frame inside HTTP 200
// and as a plain HTTP 429 payload, so both shapes are accepted here.
func parseErrorBody(body []byte) *UpstreamError {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '{' {
		var frame struct {
			Error any `json:"error"`
		}
		if common.UnmarshalJsonStr(string(trimmed), &frame) == nil {
			if frame.Error != nil {
				return parseUpstreamError(frame.Error)
			}
			var generic map[string]any
			if common.UnmarshalJsonStr(string(trimmed), &generic) == nil {
				if _, hasMessage := generic["message"]; hasMessage {
					return parseUpstreamError(generic)
				}
				if _, hasCode := generic["code"]; hasCode {
					return parseUpstreamError(generic)
				}
			}
			return nil
		}
	}
	return parseUpstreamError(string(trimmed))
}

// readErrorBody buffers a failed upstream response so the quota window it
// reports can be recorded, then rebuilds the body so the shared relay error
// path still sees the original message.
func readErrorBody(resp *http.Response) (*UpstreamError, error) {
	buffered, err := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if len(buffered) > errorBodyLimit {
		resp.Body = &prefixedBody{
			Reader: io.MultiReader(bytes.NewReader(buffered), resp.Body),
			Closer: resp.Body,
		}
		resp.ContentLength = -1
		resp.Header.Del("Content-Length")
	} else {
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(buffered))
		resp.ContentLength = int64(len(buffered))
		resp.Header.Del("Content-Length")
	}
	return parseErrorBody(buffered), nil
}

// peekFirstFrameError inspects the first SSE event before any byte reaches the
// client. Promotional routes report exhausted free quota as an error frame in a
// started stream, which can only become a real HTTP status while nothing has
// been written downstream yet.
func peekFirstFrameError(body io.Reader) (*UpstreamError, []byte, error) {
	const peekLimit = 64 << 10
	buffered := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		if eventEnd := firstEventEnd(buffered); eventEnd >= 0 {
			first := buffered[:eventEnd]
			if err := firstEventError(first); err != nil {
				return err, buffered, nil
			}
			return nil, buffered, nil
		}
		if len(buffered) > 0 && bytes.HasSuffix(buffered, []byte("\n")) && bytes.Count(buffered, []byte("\n")) == 1 {
			if err := firstEventError(buffered); err != nil {
				return err, buffered, nil
			}
		}
		if len(buffered) >= peekLimit {
			return nil, buffered, nil
		}
		n, err := body.Read(chunk)
		if n > 0 {
			buffered = append(buffered, chunk[:n]...)
			continue
		}
		if err != nil {
			if err == io.EOF {
				return nil, buffered, nil
			}
			return nil, buffered, err
		}
	}
}

// firstEventEnd reports where the first SSE event block ends, accepting both
// LF and CRLF separators, or -1 when the block is still incomplete.
func firstEventEnd(buffered []byte) int {
	index := -1
	for _, separator := range [][]byte{[]byte("\n\n"), []byte("\r\n\r\n")} {
		if at := bytes.Index(buffered, separator); at >= 0 && (index < 0 || at < index) {
			index = at
		}
	}
	return index
}

func firstEventError(event []byte) *UpstreamError {
	for _, line := range strings.Split(string(event), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var frame struct {
			Error any `json:"error"`
		}
		if common.UnmarshalJsonStr(data, &frame) != nil || frame.Error == nil {
			continue
		}
		return parseUpstreamError(frame.Error)
	}
	return nil
}

type collectedChoice struct {
	Index        int              `json:"index"`
	Message      collectedMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
	tools        map[int]*dto.ToolCallResponse
}
type collectedMessage struct {
	Role      string                  `json:"role"`
	Content   string                  `json:"content"`
	Reasoning string                  `json:"reasoning_content,omitempty"`
	Tools     []*dto.ToolCallResponse `json:"tool_calls,omitempty"`
}

func collectStream(body io.Reader) ([]byte, error) {
	result := struct {
		ID      string             `json:"id"`
		Object  string             `json:"object"`
		Created int64              `json:"created"`
		Model   string             `json:"model"`
		Choices []*collectedChoice `json:"choices"`
		Usage   *dto.Usage         `json:"usage,omitempty"`
	}{Object: "chat.completion"}
	choices := map[int]*collectedChoice{}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	total := 0
	done := false
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line)
		if total > 32<<20 {
			return nil, fmt.Errorf("Cline response too large")
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			done = true
			break
		}
		if data == "" {
			continue
		}
		var frame struct {
			dto.ChatCompletionsStreamResponse
			Error any `json:"error"`
		}
		if common.UnmarshalJsonStr(data, &frame) != nil {
			return nil, fmt.Errorf("Invalid Cline SSE frame")
		}
		if frame.Error != nil {
			return nil, parseUpstreamError(frame.Error)
		}
		if frame.Id != "" {
			result.ID = frame.Id
		}
		if frame.Model != "" {
			result.Model = frame.Model
		}
		if frame.Created != 0 {
			result.Created = frame.Created
		}
		if frame.Usage != nil {
			result.Usage = frame.Usage
		}
		for _, delta := range frame.Choices {
			c := choices[delta.Index]
			if c == nil {
				c = &collectedChoice{Index: delta.Index, Message: collectedMessage{Role: "assistant"}, tools: map[int]*dto.ToolCallResponse{}}
				choices[delta.Index] = c
			}
			c.Message.Content += delta.Delta.GetContentString()
			c.Message.Reasoning += delta.Delta.GetReasoningContent()
			if delta.FinishReason != nil {
				c.FinishReason = *delta.FinishReason
			}
			for _, tool := range delta.Delta.ToolCalls {
				if tool.Index == nil || *tool.Index < 0 {
					return nil, fmt.Errorf("Invalid Cline tool index")
				}
				index := *tool.Index
				t := c.tools[index]
				if t == nil {
					t = &dto.ToolCallResponse{Type: "function"}
					c.tools[index] = t
				}
				if tool.ID != "" {
					t.ID = tool.ID
				}
				t.Function.Name += tool.Function.Name
				t.Function.Arguments += tool.Function.Arguments
			}
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("Cline stream read failed")
	}
	if !done || len(choices) == 0 {
		return nil, fmt.Errorf("Incomplete Cline stream")
	}
	for _, c := range choices {
		if c.FinishReason == "" {
			return nil, fmt.Errorf("Incomplete Cline choice")
		}
		indices := make([]int, 0, len(c.tools))
		for i := range c.tools {
			indices = append(indices, i)
		}
		sort.Ints(indices)
		for _, i := range indices {
			c.Message.Tools = append(c.Message.Tools, c.tools[i])
		}
		result.Choices = append(result.Choices, c)
	}
	sort.Slice(result.Choices, func(i, j int) bool { return result.Choices[i].Index < result.Choices[j].Index })
	return common.Marshal(result)
}
