package workbuddy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ErrEmptyStream reports an upstream answer that carried no usable frame.
var ErrEmptyStream = errors.New("workbuddy upstream stream contained no data frame")

// StreamRetry opens one more upstream attempt for the same request. The caller
// closes the body it replaces.
type StreamRetry func() (io.ReadCloser, error)

// NormalizeStream rewrites the upstream server sent events into the standard
// shape: only known fields survive, empty deltas and placeholder tool calls are
// dropped, and the stream always ends with a done marker.
func NormalizeStream(src io.Reader) io.Reader {
	return newNormalizeReader(src, nil)
}

// NormalizeStreamWithRetry rewrites the stream and asks for one more upstream
// attempt when the answer turns out to be unusable. The terminal frames of the
// discarded attempt are dropped, so a retried turn shows a single answer.
func NormalizeStreamWithRetry(src io.Reader, retry StreamRetry) io.Reader {
	return newNormalizeReader(src, retry)
}

func newNormalizeReader(src io.Reader, retry StreamRetry) *normalizeReader {
	reader := &normalizeReader{src: bufio.NewReaderSize(src, 64*1024), retry: retry, attempt: 1, maxAttempts: 1}
	if retry != nil {
		reader.maxAttempts = 2
	}
	return reader
}

type normalizeReader struct {
	src           *bufio.Reader
	buffer        bytes.Buffer
	seenID        string
	toolCallNames map[int]bool
	// pendingToolCalls holds tool call fragments that arrived before the
	// function name. Strict clients register the call when its first fragment
	// arrives, so a name that shows up later must still be delivered with that
	// first fragment.
	toolCallPending  map[int][]map[string]any
	toolCallEnvelope map[int]map[string]any
	// toolCallIndexByID keeps one index per call id for upstreams that omit
	// the index on later fragments; toolCallIDByIndex is the reverse view and
	// detects an index the upstream reused for a second call.
	toolCallIndexByID map[string]int
	toolCallIDByIndex map[int]string
	nextToolCallIndex int
	// lastToolCallIndex is the call that is currently open, so a fragment
	// without any identity continues it instead of opening a new call.
	lastToolCallIndex int
	// answers counts the frames that carried an answer (text or a tool call)
	// and lastPayload keeps the most recent raw frame for diagnosis.
	answers int
	// reported guards the single diagnostic line per attempt.
	reported bool
	// retry asks for one more upstream attempt when an attempt ends without a
	// usable answer; maxAttempts bounds the attempts.
	retry       StreamRetry
	attempt     int
	maxAttempts int
	// tail holds the terminal frames (finish reason, upstream error frame, done
	// marker) until the answer is known, so a discarded attempt leaves nothing
	// behind on the client.
	tail bytes.Buffer
	// pending holds frames that render nothing until something meaningful
	// follows; they are dropped together with a discarded attempt.
	pending bytes.Buffer
	sawDone bool
	// forwarded counts the frames the client really receives with usable
	// content: text, or a tool call whose function name is known. A stream
	// whose raw frames carried tool calls but forwarded none leaves a strict
	// client with an empty answer, so it is reported as a failure.
	forwarded int
	// toolFrames keeps the first tool call frames of one stream, so a call the
	// client rejected can be inspected in the log.
	toolFrames  []string
	lastPayload string
	done        bool
	err         error
}

func (r *normalizeReader) Read(p []byte) (int, error) {
	for r.buffer.Len() == 0 {
		if r.err != nil {
			return 0, r.err
		}
		r.fill()
	}
	return r.buffer.Read(p)
}

func (r *normalizeReader) fill() {
	line, err := r.src.ReadString('\n')
	if err != nil && line == "" {
		if err == io.EOF {
			r.finishStream()
			return
		}
		r.err = err
		return
	}
	trimmed := strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(strings.TrimSpace(trimmed), "data:") {
		// Comments and blank separators carry no payload; the rewritten frames
		// bring their own separators.
		if err == io.EOF {
			r.finishStream()
		}
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(trimmed), "data:"))
	if len(payload) > 0 {
		r.lastPayload = payload
	}
	if payload == "[DONE]" {
		r.sawDone = true
		r.tail.WriteString("data: [DONE]\n\n")
		r.finishStream()
		return
	}
	for _, frame := range r.rewrite(payload) {
		switch {
		case frame.terminal:
			writeFrame(&r.tail, frame.payload)
		case !frame.meaningful:
			// A frame that renders nothing (role only, empty delta) waits until
			// something meaningful follows, so a discarded attempt leaves no
			// frames at all on the client.
			writeFrame(&r.pending, frame.payload)
		default:
			r.flushPending()
			writeFrame(&r.buffer, frame.payload)
		}
	}
	if err == io.EOF {
		r.finishStream()
	}
}

// finishStream closes one upstream attempt. An attempt that carried no usable
// answer is retried once when a retry source is configured; the held terminal
// frames of that attempt are dropped, so the client only sees the answer that
// survives. Without a retry, or after the last attempt, a silent end is
// reported as a failure because strict clients would otherwise show a generic
// disconnect message.
func (r *normalizeReader) finishStream() {
	if r.done {
		return
	}
	r.reportStream()
	if r.forwarded == 0 && r.retry != nil && r.attempt < r.maxAttempts {
		next, err := r.retry()
		if err == nil && next != nil {
			r.restart(next)
			return
		}
		if err != nil {
			common.SysError(fmt.Sprintf("workbuddy relay retry failed: %v", err))
		}
	}
	r.flushPending()
	if r.forwarded == 0 {
		common.SysError(fmt.Sprintf("workbuddy upstream stream ended without an answer, last frame: %s", truncateFrame(r.lastPayload)))
		r.buffer.WriteString("data: {\"error\":{\"message\":\"upstream stream ended without an answer\",\"type\":\"upstream_error\"}}\n\n")
	}
	r.done = true
	r.buffer.Write(r.tail.Bytes())
	r.tail.Reset()
	if !r.sawDone {
		r.buffer.WriteString("data: [DONE]\n\n")
	}
	r.err = io.EOF
}

// restart switches to another upstream attempt. Everything the discarded
// attempt held is dropped and the per-stream state starts over.
func (r *normalizeReader) restart(next io.ReadCloser) {
	r.src = bufio.NewReaderSize(next, 64*1024)
	r.attempt++
	r.tail.Reset()
	r.pending.Reset()
	r.sawDone = false
	r.reported = false
	r.answers = 0
	r.forwarded = 0
	r.toolFrames = nil
	r.toolCallNames = map[int]bool{}
	r.toolCallPending = nil
	r.toolCallEnvelope = nil
	r.toolCallIndexByID = map[string]int{}
	r.toolCallIDByIndex = map[int]string{}
	r.nextToolCallIndex = 0
	r.lastToolCallIndex = 0
	r.seenID = ""
	r.lastPayload = ""
}

// recordToolFrame keeps the first fragments of a stream that carry a function
// name, up to three, for diagnosis.
func (r *normalizeReader) recordToolFrame(call map[string]any) {
	if len(r.toolFrames) >= 3 {
		return
	}
	raw, err := json.Marshal(call)
	if err != nil {
		return
	}
	r.toolFrames = append(r.toolFrames, string(raw))
}

// reportStream writes one line for every stream that carried tool calls or
// dropped fragments. The line is emitted when the stream ends, either at the
// upstream [DONE] marker or at end of body, because the reader is not asked for
// more data after [DONE] and a tail-only report would never run.
func (r *normalizeReader) reportStream() {
	if r.reported {
		return
	}
	r.reported = true
	if len(r.toolFrames) == 0 && len(r.toolCallPending) == 0 && !(r.answers > 0 && r.forwarded == 0) {
		return
	}
	common.SysError(fmt.Sprintf(
		"workbuddy relay stream: attempt=%d answers=%d forwarded=%d calls=%s held=%s last=%s",
		r.attempt, r.answers, r.forwarded, truncateFrame(strings.Join(r.toolFrames, " ")), truncateFrame(r.heldPayload()), truncateFrame(r.lastPayload)))
}

// heldPayload renders the tool call fragments that never received a name, so a
// dropped answer can be diagnosed from the log alone.
func (r *normalizeReader) heldPayload() string {
	if len(r.toolCallPending) == 0 {
		return ""
	}
	indexes := make([]int, 0, len(r.toolCallPending))
	for index := range r.toolCallPending {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	fragments := make([]any, 0, len(indexes))
	for _, index := range indexes {
		for _, call := range r.toolCallPending[index] {
			fragments = append(fragments, call)
		}
	}
	raw, err := json.Marshal(fragments)
	if err != nil {
		return ""
	}
	return string(raw)
}

// frameCarriesAnswer reports whether one frame holds something a client can
// use: text content, or a tool call whose name is already known.
func frameCarriesAnswer(frame map[string]any) bool {
	choices, ok := frame["choices"].([]any)
	if !ok {
		return false
	}
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		source, ok := toolCallSource(choice)
		if !ok {
			continue
		}
		if text, ok := source["content"].(string); ok && strings.TrimSpace(text) != "" {
			return true
		}
		calls, ok := source["tool_calls"].([]any)
		if !ok {
			continue
		}
		for _, rawCall := range calls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			if function, ok := call["function"].(map[string]any); ok {
				if name, ok := function["name"].(string); ok && strings.TrimSpace(name) != "" {
					return true
				}
			}
			if name, ok := call["name"].(string); ok && strings.TrimSpace(name) != "" {
				return true
			}
		}
	}
	return false
}

// countAnswer records whether a frame carried text or a tool call.
func (r *normalizeReader) countAnswer(frame map[string]any) {
	choices, ok := frame["choices"].([]any)
	if !ok {
		return
	}
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		source, ok := toolCallSource(choice)
		if !ok {
			continue
		}
		if text, ok := source["content"].(string); ok && strings.TrimSpace(text) != "" {
			r.answers++
			return
		}
		if calls, ok := source["tool_calls"].([]any); ok && len(calls) > 0 {
			r.answers++
			return
		}
	}
}

func truncateFrame(payload string) string {
	if len(payload) <= 400 {
		return payload
	}
	return payload[:400] + "..."
}

func (r *normalizeReader) rewrite(payload string) []normalizedFrame {
	var frame map[string]any
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		return []normalizedFrame{{payload: payload}}
	}
	if _, isError := frame["error"]; isError {
		// Error frames keep their original fields so the client sees the
		// upstream code and message. They end the answer and are therefore
		// held like a finish frame.
		return []normalizedFrame{{payload: payload, terminal: true}}
	}
	if r.seenID == "" {
		if id, ok := frame["id"].(string); ok && id != "" {
			r.seenID = id
		}
	} else if id, ok := frame["id"].(string); !ok || id == "" {
		frame["id"] = r.seenID
	}
	if r.toolCallNames == nil {
		r.toolCallNames = map[int]bool{}
	}
	r.countAnswer(frame)
	released := r.splitToolCalls(frame)
	rewritten := make([]normalizedFrame, 0, 2)
	if released != nil {
		r.forwarded++
		if out, err := json.Marshal(normalizeFrame(released)); err == nil {
			// A released frame always carries a named tool call, so it renders
			// on the client and goes out at once.
			rewritten = append(rewritten, normalizedFrame{payload: string(out), meaningful: true})
		}
	}
	if frameCarriesAnswer(frame) {
		r.forwarded++
	}
	out, err := json.Marshal(normalizeFrame(frame))
	if err != nil {
		return []normalizedFrame{{payload: payload}}
	}
	return append(rewritten, normalizedFrame{
		payload:    string(out),
		terminal:   frameIsTerminal(frame),
		meaningful: frameIsMeaningful(frame),
	})
}

// flushPending sends the frames that render nothing, so the client sees them in
// front of the frame that follows.
func (r *normalizeReader) flushPending() {
	if r.pending.Len() == 0 {
		return
	}
	r.buffer.Write(r.pending.Bytes())
	r.pending.Reset()
}

// writeFrame appends one server sent event frame to a buffer.
func writeFrame(target *bytes.Buffer, payload string) {
	target.WriteString("data: ")
	target.WriteString(payload)
	target.WriteString("\n\n")
}

// frameIsMeaningful reports whether a frame renders something on the client.
func frameIsMeaningful(frame map[string]any) bool {
	choices, ok := frame["choices"].([]any)
	if !ok {
		return false
	}
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
			return true
		}
		source, ok := toolCallSource(choice)
		if !ok {
			continue
		}
		for _, key := range []string{"content", "reasoning_content", "refusal"} {
			if text, ok := source[key].(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
		if calls, ok := source["tool_calls"].([]any); ok && len(calls) > 0 {
			return true
		}
		if call, ok := source["function_call"].(map[string]any); ok && len(call) > 0 {
			return true
		}
	}
	return false
}

// normalizedFrame is one rewritten frame. Terminal frames close the answer and
// wait in the tail until the answer is known to be usable.
type normalizedFrame struct {
	payload    string
	terminal   bool
	meaningful bool
}

// frameIsTerminal reports whether a frame carries the finish reason.
func frameIsTerminal(frame map[string]any) bool {
	choices, ok := frame["choices"].([]any)
	if !ok {
		return false
	}
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
			return true
		}
	}
	return false
}

// HasUsableAnswer reports whether an aggregated chat completion carries text or
// a tool call. The relay uses it to retry an upstream that answered with an
// empty turn.
func HasUsableAnswer(raw []byte) bool {
	var answer struct {
		Choices []struct {
			Message struct {
				Content   *string `json:"content"`
				ToolCalls []any   `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return false
	}
	for _, choice := range answer.Choices {
		if choice.Message.Content != nil && strings.TrimSpace(*choice.Message.Content) != "" {
			return true
		}
		if len(choice.Message.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

// splitToolCalls normalizes the tool call fragments of one frame and returns an
// extra frame when held fragments can finally be sent.
//
// Upstream sends a tool call as fragments: the first one carries the identity
// and usually the function name, the ones after it carry argument pieces. Two
// upstream habits break strict clients:
//
//   - the first fragment may arrive without a name, with the name following in
//     a later fragment. A client that registers the call when it sees the first
//     fragment would keep an empty name, which strict clients report as a call
//     to an undeclared tool. Such fragments are therefore held back until the
//     name arrives, and then sent together with it.
//   - an empty name key must never be forwarded, because clients that treat an
//     empty string as a name also report an undeclared tool.
//
// A name that never arrives is dropped together with its held fragments: an
// argument-only fragment is unusable, and sending it makes a strict client fail
// the whole turn.
func (r *normalizeReader) splitToolCalls(frame map[string]any) map[string]any {
	choices, ok := frame["choices"].([]any)
	if !ok {
		return nil
	}
	var released map[string]any
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		delta, ok := toolCallSource(choice)
		if !ok {
			continue
		}
		calls, ok := delta["tool_calls"].([]any)
		if !ok || len(calls) == 0 {
			continue
		}
		kept := make([]any, 0, len(calls))
		for _, rawCall := range calls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				kept = append(kept, rawCall)
				continue
			}
			index := r.resolveToolCallIndex(call)
			r.lastToolCallIndex = index
			if value, ok := call["index"].(float64); !ok || int(value) != index {
				// Fragments without an index are matched by their id, and an
				// index the upstream reused for another call is moved to a free
				// slot; either way the resolved index travels with the fragment
				// so the client merges the pieces of one call.
				call["index"] = index
			}
			function, _ := call["function"].(map[string]any)
			name := ""
			if function != nil {
				name, _ = function["name"].(string)
			}
			if name == "" {
				name, _ = call["name"].(string)
			}
			name = strings.TrimSpace(name)
			switch {
			case name == "":
				if function != nil {
					delete(function, "name")
				}
				if !r.toolCallNames[index] {
					r.holdToolCall(frame, index, call)
					continue
				}
				kept = append(kept, call)
			case r.toolCallNames[index]:
				delete(function, "name")
				kept = append(kept, call)
			default:
				r.recordToolFrame(call)
				r.toolCallNames[index] = true
				held := r.releaseToolCall(index)
				if len(held) == 0 {
					kept = append(kept, call)
					continue
				}
				// Only the name travels with the held fragments; the arguments of
				// this fragment stay on it, so they are not sent twice.
				merged := mergeToolCallFragments(append(held, map[string]any{
					"index":    index,
					"type":     "function",
					"function": map[string]any{"name": name},
				}))
				if candidate := frameWithToolCalls(frame, merged); candidate != nil {
					released = candidate
				}
				delete(function, "name")
				kept = append(kept, call)
			}
		}
		if len(kept) == 0 {
			delete(delta, "tool_calls")
			continue
		}
		delta["tool_calls"] = kept
	}
	return released
}

// resolveToolCallIndex returns the index of a fragment. Upstreams that omit the
// index are matched by call id, so parallel calls do not collapse into one.
func (r *normalizeReader) resolveToolCallIndex(call map[string]any) int {
	if r.toolCallIndexByID == nil {
		r.toolCallIndexByID = map[string]int{}
	}
	if r.toolCallIDByIndex == nil {
		r.toolCallIDByIndex = map[int]string{}
	}
	id, _ := call["id"].(string)
	if value, ok := call["index"].(float64); ok {
		index := int(value)
		if id == "" {
			return index
		}
		if known, seen := r.toolCallIndexByID[id]; seen {
			return known
		}
		if owner, taken := r.toolCallIDByIndex[index]; taken && owner != id {
			// The upstream restarted its numbering for a new call. Keeping the
			// old index would make the new call look like a repeat, and its
			// function name would be stripped as a duplicate.
			index = r.freshToolCallIndex()
		}
		r.toolCallIndexByID[id] = index
		r.toolCallIDByIndex[index] = id
		if index >= r.nextToolCallIndex {
			r.nextToolCallIndex = index + 1
		}
		return index
	}
	if id != "" {
		if index, seen := r.toolCallIndexByID[id]; seen {
			return index
		}
		index := r.freshToolCallIndex()
		r.toolCallIndexByID[id] = index
		r.toolCallIDByIndex[index] = id
		return index
	}
	return r.lastToolCallIndex
}

// freshToolCallIndex returns the next index that no call occupies, so a call
// whose index was reused by the upstream does not collide with a live one.
func (r *normalizeReader) freshToolCallIndex() int {
	for {
		index := r.nextToolCallIndex
		r.nextToolCallIndex++
		if _, taken := r.toolCallIDByIndex[index]; taken {
			continue
		}
		if _, held := r.toolCallPending[index]; held {
			continue
		}
		return index
	}
}

// holdToolCall keeps a nameless fragment together with the frame envelope, so
// it can be sent once the name is known.
func (r *normalizeReader) holdToolCall(frame map[string]any, index int, call map[string]any) {
	if r.toolCallPending == nil {
		r.toolCallPending = map[int][]map[string]any{}
		r.toolCallEnvelope = map[int]map[string]any{}
	}
	r.toolCallPending[index] = append(r.toolCallPending[index], cloneToolCall(call))
	if _, ok := r.toolCallEnvelope[index]; !ok {
		if envelope := frameEnvelope(frame); envelope != nil {
			r.toolCallEnvelope[index] = envelope
		}
	}
}

// releaseToolCall returns the fragments held for one index and forgets them.
func (r *normalizeReader) releaseToolCall(index int) []map[string]any {
	held := r.toolCallPending[index]
	delete(r.toolCallPending, index)
	delete(r.toolCallEnvelope, index)
	return held
}

// frameEnvelope copies a frame without its choices.
func frameEnvelope(frame map[string]any) map[string]any {
	envelope := map[string]any{}
	for key, value := range frame {
		if key == "choices" {
			continue
		}
		envelope[key] = value
	}
	return envelope
}

// frameWithToolCalls rebuilds a frame from an envelope and one tool call.
func frameWithToolCalls(frame map[string]any, call map[string]any) map[string]any {
	rebuilt := frameEnvelope(frame)
	rebuilt["choices"] = []any{map[string]any{
		"index":         0,
		"delta":         map[string]any{"tool_calls": []any{call}},
		"finish_reason": nil,
	}}
	return rebuilt
}

// mergeToolCallFragments folds the held fragments and the fragment carrying the
// name into one tool call.
func mergeToolCallFragments(fragments []map[string]any) map[string]any {
	merged := map[string]any{"type": "function", "function": map[string]any{}}
	function := merged["function"].(map[string]any)
	arguments := ""
	for _, fragment := range fragments {
		for key, value := range fragment {
			if key == "function" {
				continue
			}
			if _, exists := merged[key]; !exists {
				merged[key] = value
			}
		}
		inner, ok := fragment["function"].(map[string]any)
		if !ok {
			continue
		}
		if name, ok := inner["name"].(string); ok && strings.TrimSpace(name) != "" {
			function["name"] = strings.TrimSpace(name)
		}
		if chunk, ok := inner["arguments"].(string); ok {
			arguments += chunk
		}
	}
	if arguments != "" {
		function["arguments"] = arguments
	}
	return merged
}

// cloneToolCall copies one fragment, so held data is not shared with the frame
// that is written out afterwards.
func cloneToolCall(call map[string]any) map[string]any {
	raw, err := json.Marshal(call)
	if err != nil {
		return call
	}
	var clone map[string]any
	if err := json.Unmarshal(raw, &clone); err != nil {
		return call
	}
	return clone
}

// toolCallSource returns the fields that carry the answer. Most frames use
// delta; some upstream frames deliver the whole message instead, and dropping
// those would leave the client with an empty answer.
func toolCallSource(choice map[string]any) (map[string]any, bool) {
	if delta, ok := choice["delta"].(map[string]any); ok && len(delta) > 0 {
		return delta, true
	}
	if message, ok := choice["message"].(map[string]any); ok && len(message) > 0 {
		return message, true
	}
	return nil, false
}

// normalizeFrame rebuilds one frame with only the fields the OpenAI streaming
// shape defines.
func normalizeFrame(frame map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"id", "object", "created", "model", "system_fingerprint", "service_tier"} {
		if value, ok := frame[key]; ok && value != nil {
			out[key] = value
		}
	}
	if _, ok := out["object"]; !ok {
		out["object"] = "chat.completion.chunk"
	}
	if _, ok := out["id"]; !ok {
		out["id"] = "chatcmpl-workbuddy"
	}
	if choices, ok := frame["choices"].([]any); ok {
		normalized := make([]any, 0, len(choices))
		for _, rawChoice := range choices {
			choice, ok := rawChoice.(map[string]any)
			if !ok {
				continue
			}
			current := map[string]any{}
			if index, ok := choice["index"]; ok {
				current["index"] = index
			}
			delta := map[string]any{}
			if rawDelta, ok := toolCallSource(choice); ok {
				if value, ok := rawDelta["role"].(string); ok && value != "" {
					delta["role"] = value
				}
				if value, ok := rawDelta["content"].(string); ok && value != "" {
					delta["content"] = value
				}
				if value, ok := rawDelta["reasoning_content"].(string); ok && value != "" {
					delta["reasoning_content"] = value
				}
				if value, ok := rawDelta["refusal"].(string); ok && value != "" {
					delta["refusal"] = value
				}
				if calls, ok := rawDelta["tool_calls"].([]any); ok && len(calls) > 0 {
					delta["tool_calls"] = calls
				}
				if functionCall, ok := rawDelta["function_call"]; ok && functionCall != nil {
					// A placeholder with empty name and arguments carries nothing.
					keep := true
					if legacy, ok := functionCall.(map[string]any); ok {
						name, _ := legacy["name"].(string)
						arguments, _ := legacy["arguments"].(string)
						keep = name != "" || arguments != ""
					}
					if keep {
						delta["function_call"] = functionCall
					}
				}
			}
			current["delta"] = delta
			if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
				current["finish_reason"] = reason
			} else {
				current["finish_reason"] = nil
			}
			normalized = append(normalized, current)
		}
		out["choices"] = normalized
	}
	if usage, ok := frame["usage"]; ok {
		out["usage"] = usage
	} else {
		out["usage"] = nil
	}
	return out
}

// AggregateStream folds a normalized stream into one non-streaming chat
// completion answer, which is what a client asking for a plain response needs.
func AggregateStream(src io.Reader) ([]byte, error) {
	reader := bufio.NewReaderSize(src, 64*1024)
	var (
		id           string
		model        string
		created      any
		content      strings.Builder
		reasoning    strings.Builder
		finishReason = "stop"
		usage        map[string]any
		frames       int
	)
	toolCalls := map[int]map[string]any{}
	var toolOrder []int
	nextIndex := 0
	indexByID := map[string]int{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
		if !strings.HasPrefix(trimmed, "data:") {
			if err == io.EOF {
				break
			}
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if payload == "[DONE]" {
			break
		}
		var frame map[string]any
		if json.Unmarshal([]byte(payload), &frame) != nil {
			continue
		}
		if upstreamErr, ok := frame["error"]; ok {
			encoded, _ := json.Marshal(upstreamErr)
			return nil, fmt.Errorf("upstream error frame: %s", string(encoded))
		}
		frames++
		if value, ok := frame["id"].(string); ok && value != "" && id == "" {
			id = value
		}
		if value, ok := frame["model"].(string); ok && value != "" {
			model = value
		}
		if value, ok := frame["created"]; ok && created == nil {
			created = value
		}
		if value, ok := frame["usage"].(map[string]any); ok {
			usage = value
		}
		choices, ok := frame["choices"].([]any)
		if !ok {
			continue
		}
		for _, rawChoice := range choices {
			choice, ok := rawChoice.(map[string]any)
			if !ok {
				continue
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if text, ok := delta["content"].(string); ok {
					content.WriteString(text)
				}
				if text, ok := delta["reasoning_content"].(string); ok {
					reasoning.WriteString(text)
				}
				if calls, ok := delta["tool_calls"].([]any); ok {
					mergeToolCalls(toolCalls, &toolOrder, indexByID, calls, &nextIndex)
				}
			}
			if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
				finishReason = reason
			}
		}
		if err == io.EOF {
			break
		}
	}
	if frames == 0 {
		return nil, ErrEmptyStream
	}
	message := map[string]any{"role": "assistant", "content": content.String()}
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}
	if len(toolOrder) > 0 {
		calls := make([]any, 0, len(toolOrder))
		for _, index := range toolOrder {
			call := toolCalls[index]
			if function, ok := call["function"].(map[string]any); ok {
				if name, _ := function["name"].(string); strings.TrimSpace(name) == "" {
					// A call without a name cannot be executed and makes strict
					// clients fail the whole turn.
					continue
				}
			}
			calls = append(calls, call)
		}
		if len(calls) > 0 {
			message["tool_calls"] = calls
		}
	}
	if id == "" {
		id = "chatcmpl-workbuddy"
	}
	if created == nil {
		created = float64(0)
	}
	answer := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
	}
	if usage != nil {
		answer["usage"] = ensureUsageTotal(usage)
	}
	return json.Marshal(answer)
}

// mergeToolCalls accumulates the streamed tool call fragments. Fragments
// without an index are matched by their id, and unknown ones start a new call.
func mergeToolCalls(acc map[int]map[string]any, order *[]int, indexByID map[string]int, calls []any, nextIndex *int) {
	for _, raw := range calls {
		call, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		index := -1
		if value, ok := call["index"].(float64); ok {
			index = int(value)
		} else if id, ok := call["id"].(string); ok && id != "" {
			if known, seen := indexByID[id]; seen {
				index = known
			} else {
				index = allocateIndex(acc, nextIndex)
			}
		} else if len(*order) > 0 {
			index = (*order)[len(*order)-1]
		} else {
			index = allocateIndex(acc, nextIndex)
		}
		merged, seen := acc[index]
		if !seen {
			merged = map[string]any{"index": index, "type": "function", "function": map[string]any{}}
			acc[index] = merged
			*order = append(*order, index)
		}
		if id, ok := call["id"].(string); ok && id != "" {
			indexByID[id] = index
			merged["id"] = id
		}
		function, ok := call["function"].(map[string]any)
		if !ok {
			continue
		}
		target, _ := merged["function"].(map[string]any)
		if target == nil {
			target = map[string]any{}
			merged["function"] = target
		}
		if name, ok := function["name"].(string); ok && name != "" {
			target["name"] = name
		}
		if arguments, ok := function["arguments"].(string); ok {
			existing, _ := target["arguments"].(string)
			target["arguments"] = existing + arguments
		}
	}
}

func allocateIndex(acc map[int]map[string]any, nextIndex *int) int {
	for {
		index := *nextIndex
		*nextIndex = index + 1
		if _, used := acc[index]; !used {
			return index
		}
	}
}

// ensureUsageTotal fills total_tokens when the upstream reports the prompt and
// completion counts only.
func ensureUsageTotal(usage map[string]any) map[string]any {
	if _, ok := usage["total_tokens"]; ok {
		return usage
	}
	prompt, okPrompt := usage["prompt_tokens"].(float64)
	completion, okCompletion := usage["completion_tokens"].(float64)
	if !okPrompt || !okCompletion {
		return usage
	}
	usage["total_tokens"] = prompt + completion
	return usage
}

// SortedToolIndexes is a helper for tests and callers that need a stable order.
func SortedToolIndexes(indexes map[int]map[string]any) []int {
	out := make([]int, 0, len(indexes))
	for index := range indexes {
		out = append(out, index)
	}
	sort.Ints(out)
	return out
}
