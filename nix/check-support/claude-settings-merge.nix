{ pkgs }:

let
  fakeFence = pkgs.writeShellScriptBin "fence" ''
    if [ "$1" = "--claude-pre-tool-use" ]; then
      : "''${DEN_TEST_FENCE_MARKER:?}"
      touch "$DEN_TEST_FENCE_MARKER"
      cat >/dev/null
      exit 0
    fi
    exit 1
  '';
  mkClaude = import ../lib/mk-claude.nix {
    fence = fakeFence;
    isDarwin = true;
    inherit pkgs;
    mkAgentSandbox = value: value;
  };
  adapter = mkClaude { };
in
pkgs.runCommand "claude-settings-merge"
  {
    nativeBuildInputs = [ pkgs.claude-code pkgs.python3 pkgs.coreutils ];
    settings = adapter.agent.darwinSettings;
  }
  ''
    set -eu
    root="$TMPDIR/claude-settings-merge"
    config="$root/config"
    mkdir -p "$config"
    fenceMarker="$root/fence-ran"
    userMarker="$root/user-hook-ran"
    fixtureLog="$root/fixture.log"
    export HOME="$root/home"
    export CLAUDE_CONFIG_DIR="$config"
    export DEN_TEST_FENCE_MARKER="$fenceMarker"
    export DEN_TEST_USER_MARKER="$userMarker"
    export ANTHROPIC_API_KEY="test-key"
    mkdir -p "$HOME"

    cat > "$config/settings.json" <<'JSON'
    {"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"touch $DEN_TEST_USER_MARKER"}]}]}}
    JSON
    cat > fixture.py <<'PYTHON'
    import json
    import sys
    from http.server import BaseHTTPRequestHandler, HTTPServer

    class Handler(BaseHTTPRequestHandler):
        count = 0
        def log_message(self, format, *args):
            pass
        def do_POST(self):
            if self.client_address[0] not in ("127.0.0.1", "::1"):
                raise RuntimeError("external request")
            length = int(self.headers["content-length"])
            request = json.loads(self.rfile.read(length))
            Handler.count += 1
            if Handler.count == 1:
                content = [{"type":"tool_use", "id":"toolu_fixture", "name":"Bash", "input":{"command":"echo fixture"}}]
                stop_reason = "tool_use"
            else:
                if not any(item.get("type") == "tool_result" for message in request.get("messages", []) for item in message.get("content", []) if isinstance(item, dict)):
                    self.send_error(400, "missing tool result")
                    return
                content = [{"type":"text", "text":"fixture complete"}]
                stop_reason = "end_turn"
            response = {"id":"msg_fixture", "type":"message", "role":"assistant", "model":"claude-3-5-haiku-latest", "content":content, "stop_reason":stop_reason, "stop_sequence":None, "usage":{"input_tokens":1, "output_tokens":1}}
            encoded = json.dumps(response).encode()
            self.send_response(200)
            self.send_header("content-type", "application/json")
            self.send_header("content-length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

    HTTPServer(("127.0.0.1", 18765), Handler).serve_forever()
    PYTHON
    ${pkgs.python3}/bin/python fixture.py > "$fixtureLog" 2>&1 &
    fixturePID=$!
    trap 'kill "$fixturePID" 2>/dev/null || true' EXIT
    sleep 1
    export ANTHROPIC_BASE_URL="http://127.0.0.1:18765"
    if ! ${pkgs.claude-code}/bin/claude -p "run the fixture command" --settings "$settings" > "$root/claude.out" 2> "$root/claude.err"; then
      cat "$root/claude.err" >&2
      cat "$fixtureLog" >&2
      exit 1
    fi
    if ! test -e "$fenceMarker" || ! test -e "$userMarker"; then
      cat "$root/claude.out" >&2
      cat "$root/claude.err" >&2
      exit 1
    fi
    touch "$out"
  ''
