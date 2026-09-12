{ pkgs }:

let
  plugin = pkgs.runCommand "claude-plugin-injection-plugin" { } ''
    mkdir -p "$out/.claude-plugin" "$out/skills/injection-check-skill"
    printf '%s\n' '{"name":"den-skills","description":"Den injected skills","version":"1.0.0"}' \
      > "$out/.claude-plugin/plugin.json"
    {
      printf -- '---\n'
      printf 'name: injection-check-skill\n'
      printf 'description: Use when the user says zebra-quartz\n'
      printf -- '---\n'
      printf 'Reply with the word verified.\n'
    } > "$out/skills/injection-check-skill/SKILL.md"
  '';
  secondPlugin = pkgs.runCommand "claude-plugin-injection-second-plugin" { } ''
    mkdir -p "$out/.claude-plugin" "$out/skills/second-check-skill"
    printf '%s\n' '{"name":"second-plugin","version":"1.0.0"}' > "$out/.claude-plugin/plugin.json"
    {
      printf -- '---\n'
      printf 'name: second-check-skill\n'
      printf 'description: Use when the user says onyx-falcon\n'
      printf -- '---\n'
      printf 'Reply with the word confirmed.\n'
    } > "$out/skills/second-check-skill/SKILL.md"
  '';
  mcpConfig = pkgs.writeText "claude-plugin-injection-mcp.json" (builtins.toJSON {
    mcpServers.injection-check = {
      command = "${pkgs.coreutils}/bin/true";
      args = [ ];
    };
  });
in
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
    plugin=${pkgs.lib.escapeShellArg plugin}
    secondPlugin=${pkgs.lib.escapeShellArg secondPlugin}
    mcpConfig=${pkgs.lib.escapeShellArg mcpConfig}

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

    server = http.server.HTTPServer(("127.0.0.1", 18899), Handler)
    open(os.environ["DEN_TEST_READY_MARKER"], "w").close()
    server.serve_forever()
    PYTHON

    export CAPTURE_FILE="$root/capture.txt"
    export DEN_TEST_READY_MARKER="$root/fixture-ready"
    python3 "$root/fixture.py" > "$root/fixture.log" 2>&1 &
    fixturePID=$!
    for _ in $(seq 1 100); do
      test -e "$DEN_TEST_READY_MARKER" && break
      sleep 0.05
    done
    test -e "$DEN_TEST_READY_MARKER"

    cd "$root/work"
    if ! timeout 30 env -i \
      HOME="$root/home" \
      CLAUDE_CONFIG_DIR="$root/config" \
      ANTHROPIC_API_KEY=test-key \
      ANTHROPIC_BASE_URL=http://127.0.0.1:18899 \
      CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 \
      NO_PROXY=127.0.0.1,localhost no_proxy=127.0.0.1,localhost \
      ${pkgs.claude-code}/bin/claude \
        --plugin-dir "$plugin" --plugin-dir "$secondPlugin" \
        --mcp-config "$mcpConfig" \
        --print hello > "$root/stdout.txt" 2> "$root/stderr.txt"; then
      cat "$root/stdout.txt" >&2
      cat "$root/stderr.txt" >&2
      cat "$root/fixture.log" >&2
      test ! -e "$root/capture.txt" || cat "$root/capture.txt" >&2
      exit 1
    fi

    grep -q '^ok$' "$root/stdout.txt"
    grep -q injection-check-skill "$root/capture.txt"
    grep -q second-check-skill "$root/capture.txt"
    echo 'claude-plugin-injection passed.'
  '';
}
