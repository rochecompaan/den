//go:build native

package native

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type claudeScenario struct {
	id       string
	commands []string
	scratch  bool
	results  []string
}

type claudeProvider struct {
	mu        sync.Mutex
	ready     *sync.Cond
	next      int
	scenarios map[string]*claudeScenario
}

func newClaudeProvider() *claudeProvider {
	provider := &claudeProvider{scenarios: make(map[string]*claudeScenario)}
	provider.ready = sync.NewCond(&provider.mu)
	return provider
}

func (provider *claudeProvider) register(commands ...string) *claudeScenario {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.next++
	scenario := &claudeScenario{id: fmt.Sprintf("den-native-%d", provider.next), commands: commands}
	provider.scenarios[scenario.id] = scenario
	return scenario
}

func (provider *claudeProvider) registerScratch(firstCommand string) *claudeScenario {
	scenario := provider.register(firstCommand)
	scenario.scratch = true
	return scenario
}

func (scenario *claudeScenario) prompt() string {
	return "execute the scripted Bash tools for " + scenario.id
}

func (provider *claudeProvider) scenarioResults(scenario *claudeScenario) []string {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]string(nil), scenario.results...)
}

func (provider *claudeProvider) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || !strings.HasPrefix(request.URL.Path, "/v1/messages") {
		http.Error(writer, "local messages fixture only", http.StatusForbidden)
		return
	}
	var document map[string]any
	if err := json.NewDecoder(request.Body).Decode(&document); err != nil {
		http.Error(writer, "invalid local messages request", http.StatusBadRequest)
		return
	}
	encoded, _ := json.Marshal(document)
	provider.mu.Lock()
	scenario := provider.matchScenarioLocked(encoded)
	if scenario == nil {
		provider.mu.Unlock()
		http.Error(writer, "unknown local messages session", http.StatusBadRequest)
		return
	}
	results := toolResults(document)
	if len(results) > len(scenario.results) {
		scenario.results = append([]string(nil), results...)
		provider.ready.Broadcast()
	}
	content, stopReason := provider.nextContentLocked(scenario)
	provider.mu.Unlock()

	payload := streamEvents(document, content, stopReason)
	writer.Header().Set("content-type", "text/event-stream")
	writer.Header().Set("content-length", fmt.Sprint(len(payload)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(payload)
}

func (provider *claudeProvider) matchScenarioLocked(document []byte) *claudeScenario {
	for id, scenario := range provider.scenarios {
		if strings.Contains(string(document), id) {
			return scenario
		}
	}
	return nil
}

func (provider *claudeProvider) nextContentLocked(scenario *claudeScenario) (map[string]any, string) {
	index := len(scenario.results)
	if scenario.scratch && index == 1 {
		for provider.scratchReadyLocked() < 2 {
			provider.ready.Wait()
		}
		own := scratchPath(scenario.results[0])
		peer := provider.peerScratchPathLocked(scenario)
		command := fmt.Sprintf(`set -eu
 test "$TMPDIR" = %q
 test "$DEN_FENCE_TMPDIR" = %q
 test -r %q/marker
 ! cat %q/marker
 if mkdir -p "$(dirname "$DEN_NATIVE_TMP_PROBE")" 2>/dev/null; then
   ! sh -c 'printf denied > "$1"' den "$DEN_NATIVE_TMP_PROBE"
 fi
 if mkdir -p "$(dirname "$DEN_NATIVE_PRIVATE_TMP_PROBE")" 2>/dev/null; then
   ! sh -c 'printf denied > "$1"' den "$DEN_NATIVE_PRIVATE_TMP_PROBE"
 fi
 printf 'scratch:%%s\ncomplete\n' "$DEN_FENCE_TMPDIR"`, own, own, own, peer)
		return toolUse(scenario, index, command), "tool_use"
	}
	if index < len(scenario.commands) {
		return toolUse(scenario, index, scenario.commands[index]), "tool_use"
	}
	return map[string]any{"type": "text", "text": "fixture complete"}, "end_turn"
}

func (provider *claudeProvider) scratchReadyLocked() int {
	count := 0
	for _, scenario := range provider.scenarios {
		if scenario.scratch && len(scenario.results) > 0 && scratchPath(scenario.results[0]) != "" {
			count++
		}
	}
	return count
}

func (provider *claudeProvider) peerScratchPathLocked(current *claudeScenario) string {
	for _, scenario := range provider.scenarios {
		if scenario != current && scenario.scratch && len(scenario.results) > 0 {
			return scratchPath(scenario.results[0])
		}
	}
	return ""
}

func scratchPath(result string) string {
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "scratch:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "scratch:"))
		}
	}
	return ""
}

func toolResults(document map[string]any) []string {
	var results []string
	messages, _ := document["messages"].([]any)
	for _, rawMessage := range messages {
		message, _ := rawMessage.(map[string]any)
		content, _ := message["content"].([]any)
		for _, rawItem := range content {
			item, _ := rawItem.(map[string]any)
			if item["type"] != "tool_result" {
				continue
			}
			switch value := item["content"].(type) {
			case string:
				results = append(results, value)
			default:
				encoded, _ := json.Marshal(value)
				results = append(results, string(encoded))
			}
		}
	}
	return results
}

func toolUse(scenario *claudeScenario, index int, command string) map[string]any {
	return map[string]any{
		"type": "tool_use", "id": fmt.Sprintf("toolu_%s_%d", scenario.id, index),
		"name": "Bash", "input": map[string]any{"command": command},
	}
}

func streamEvents(request map[string]any, content map[string]any, stopReason string) []byte {
	model, _ := request["model"].(string)
	message := map[string]any{
		"id": "msg_den_native", "type": "message", "role": "assistant", "model": model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 0},
	}
	events := [][2]any{{"message_start", map[string]any{"type": "message_start", "message": message}}}
	if content["type"] == "tool_use" {
		events = append(events,
			[2]any{"content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{
				"type": "tool_use", "id": content["id"], "name": "Bash", "input": map[string]any{},
			}}},
			[2]any{"content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{
				"type": "input_json_delta", "partial_json": mustJSON(content["input"]),
			}}},
		)
	} else {
		events = append(events,
			[2]any{"content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}},
			[2]any{"content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": content["text"]}}},
		)
	}
	events = append(events,
		[2]any{"content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}},
		[2]any{"message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 1}}},
		[2]any{"message_stop", map[string]any{"type": "message_stop"}},
	)
	var stream strings.Builder
	for _, event := range events {
		stream.WriteString("event: ")
		stream.WriteString(event[0].(string))
		stream.WriteString("\ndata: ")
		stream.WriteString(mustJSON(event[1]))
		stream.WriteString("\n\n")
	}
	return []byte(stream.String())
}

func mustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
