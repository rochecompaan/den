{ pkgs }:

pkgs.writeShellApplication {
  name = "claude-plugin-injection";
  runtimeInputs = [ pkgs.claude-code pkgs.coreutils pkgs.gnugrep pkgs.python3 ];
  text = ''
    set -eu
    root=$(mktemp -d "''${TMPDIR:-/tmp}/claude-plugin-injection.XXXXXX")
    fixturePID=
    cleanup() {
      status=$?
      trap - EXIT
      if [ -n "$fixturePID" ]; then
        kill "$fixturePID" 2>/dev/null || true
        wait "$fixturePID" 2>/dev/null || true
      fi
      if ! rm -rf "$root" && [ "$status" -eq 0 ]; then status=1; fi
      exit "$status"
    }
    trap cleanup EXIT

    mkdir -p "$root/home" "$root/config" "$root/work"

    plugin=$root/den-skills
    mkdir -p "$plugin/.claude-plugin" "$plugin/skills/injection-check-skill"
    printf '%s\n' '{"name":"den-skills","description":"Den injected skills","version":"1.0.0"}' \
      > "$plugin/.claude-plugin/plugin.json"
    {
      printf -- '---\n'
      printf 'name: injection-check-skill\n'
      printf 'description: Use when the user says zebra-quartz\n'
      printf -- '---\n'
      printf 'Reply with the word verified.\n'
    } > "$plugin/skills/injection-check-skill/SKILL.md"

    second=$root/second-plugin
    mkdir -p "$second/.claude-plugin" "$second/skills/second-check-skill"
    printf '%s\n' '{"name":"second-plugin","version":"1.0.0"}' > "$second/.claude-plugin/plugin.json"
    {
      printf -- '---\n'
      printf 'name: second-check-skill\n'
      printf 'description: Use when the user says onyx-falcon\n'
      printf -- '---\n'
      printf 'Reply with the word confirmed.\n'
    } > "$second/skills/second-check-skill/SKILL.md"

    printf '%s\n' '{"mcpServers":{"injection-check":{"command":"${pkgs.coreutils}/bin/true","args":[]}}}' \
      > "$root/mcp.json"

    cat > "$root/fixture.py" <<'PYTHON'
    import http.server, json, os

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("content-length", 0))
            body = self.rfile.read(length)
            with open(os.environ["CAPTURE_FILE"], "ab") as capture:
                capture.write(body + b"\n---REQUEST---\n")
            response = json.dumps({
                "id": "msg_1", "type": "message", "role": "assistant",
                "model": "claude-opus-4-6",
                "content": [{"type": "text", "text": "ok"}],
                "stop_reason": "end_turn",
                "usage": {"input_tokens": 1, "output_tokens": 1},
            }).encode()
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.send_header("content-length", str(len(response)))
            self.end_headers()
            self.wfile.write(response)

        def log_message(self, *arguments):
            pass

    http.server.HTTPServer(("127.0.0.1", 18899), Handler).serve_forever()
    PYTHON

    export CAPTURE_FILE="$root/capture.txt"
    python3 "$root/fixture.py" &
    fixturePID=$!
    sleep 1

    cd "$root/work"
    env -i \
      HOME="$root/home" \
      CLAUDE_CONFIG_DIR="$root/config" \
      ANTHROPIC_API_KEY=test-key \
      ANTHROPIC_BASE_URL=http://127.0.0.1:18899 \
      CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 \
      NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
      ${pkgs.claude-code}/bin/claude \
        --plugin-dir "$plugin" --plugin-dir "$second" \
        --mcp-config "$root/mcp.json" \
        --print hello > "$root/stdout.txt"

    grep -q '^ok$' "$root/stdout.txt"
    grep -q injection-check-skill "$root/capture.txt"
    grep -q second-check-skill "$root/capture.txt"
    echo 'claude-plugin-injection passed.'
  '';
}
