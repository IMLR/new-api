package cline

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

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
			return nil, fmt.Errorf("Cline upstream stream failed")
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
