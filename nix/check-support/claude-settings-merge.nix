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
  adapter = (mkClaude { }).adapter;
  mandatoryArgs = pkgs.lib.escapeShellArgs adapter.agent.mandatoryArgs;
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
    mkdir -p "$config" "$root/home"

    export HOME="$root/home"
    export CLAUDE_CONFIG_DIR="$config"
    export DEN_TEST_FENCE_MARKER="$root/fence-ran"
    export DEN_TEST_USER_MARKER="$root/user-hook-ran"
    export DEN_TEST_BASH_MARKER="$root/bash-ran"
    export DEN_TEST_EXTERNAL_MARKER="$root/external-attempt"
    export DEN_TEST_PROTOCOL_MARKER="$root/protocol-error"
    export DEN_TEST_READY_MARKER="$root/fixture-ready"
    export DEN_TEST_TOOL_MARKER="$root/tool-requested"
    export DEN_TEST_FINAL_MARKER="$root/final-sent"

    # All credentials and endpoint overrides are local to this derivation.
    export ANTHROPIC_API_KEY="test-key"
    export ANTHROPIC_BASE_URL="http://127.0.0.1:18765"
    export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1

    # Route standard proxy-aware external traffic back to the rejecting fixture.
    export HTTP_PROXY="$ANTHROPIC_BASE_URL"
    export HTTPS_PROXY="$ANTHROPIC_BASE_URL"
    export ALL_PROXY="$ANTHROPIC_BASE_URL"
    export http_proxy="$HTTP_PROXY"
    export https_proxy="$HTTPS_PROXY"
    export all_proxy="$ALL_PROXY"
    export NO_PROXY="127.0.0.1,localhost"
    export no_proxy="$NO_PROXY"

    cat > "$config/settings.json" <<'JSON'
    {"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"touch $DEN_TEST_USER_MARKER"}]}]}}
    JSON
    settingsFingerprint=$(sha256sum "$settings" "$config/settings.json")

    cat > fixture.py <<'PYTHON'
    import json
    import os
    from http.server import BaseHTTPRequestHandler, HTTPServer

    LOOPBACK_HOST = "127.0.0.1"
    LOOPBACK_PORT = 18765
    TOOL_ID = "toolu_01denfixture000000000000"

    def mark(name):
        open(os.environ[name], "w").close()

    def has_tool_result(request):
        return any(
            item.get("type") == "tool_result"
            for message in request.get("messages", [])
            for item in message.get("content", [])
            if isinstance(item, dict)
        )

    def response_content(request):
        if has_tool_result(request):
            if not os.path.exists(os.environ["DEN_TEST_BASH_MARKER"]):
                raise ValueError("tool result arrived before fixture Bash marker")
            mark("DEN_TEST_FINAL_MARKER")
            return {"type": "text", "text": "fixture complete"}, "end_turn"

        if os.path.exists(os.environ["DEN_TEST_TOOL_MARKER"]):
            raise ValueError("received a second request without a tool result")
        mark("DEN_TEST_TOOL_MARKER")
        command = "test -e '{}' -a -e '{}' && touch '{}'".format(
            os.environ["DEN_TEST_FENCE_MARKER"],
            os.environ["DEN_TEST_USER_MARKER"],
            os.environ["DEN_TEST_BASH_MARKER"],
        )
        return {
            "type": "tool_use",
            "id": TOOL_ID,
            "name": "Bash",
            "input": {"command": command},
        }, "tool_use"

    def message(request, content, stop_reason):
        return {
            "id": "msg_01denfixture000000000000",
            "type": "message",
            "role": "assistant",
            "model": request.get("model", "claude-opus-4-8"),
            "content": [content],
            "stop_reason": stop_reason,
            "stop_sequence": None,
            "usage": {"input_tokens": 1, "output_tokens": 1},
        }

    def stream_events(request, content, stop_reason):
        start = message(request, content, None)
        start["content"] = []
        start["usage"]["output_tokens"] = 0
        events = [("message_start", {"type": "message_start", "message": start})]
        if content["type"] == "tool_use":
            events.extend([
                ("content_block_start", {
                    "type": "content_block_start",
                    "index": 0,
                    "content_block": {
                        "type": "tool_use",
                        "id": content["id"],
                        "name": content["name"],
                        "input": {},
                    },
                }),
                ("content_block_delta", {
                    "type": "content_block_delta",
                    "index": 0,
                    "delta": {
                        "type": "input_json_delta",
                        "partial_json": json.dumps(content["input"], separators=(",", ":")),
                    },
                }),
            ])
        else:
            events.extend([
                ("content_block_start", {
                    "type": "content_block_start",
                    "index": 0,
                    "content_block": {"type": "text", "text": ""},
                }),
                ("content_block_delta", {
                    "type": "content_block_delta",
                    "index": 0,
                    "delta": {"type": "text_delta", "text": content["text"]},
                }),
            ])
        events.extend([
            ("content_block_stop", {"type": "content_block_stop", "index": 0}),
            ("message_delta", {
                "type": "message_delta",
                "delta": {"stop_reason": stop_reason, "stop_sequence": None},
                "usage": {"output_tokens": 1},
            }),
            ("message_stop", {"type": "message_stop"}),
        ])
        return "".join(
            "event: {}\ndata: {}\n\n".format(
                event,
                json.dumps(data, separators=(",", ":")),
            )
            for event, data in events
        ).encode()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, format, *args):
            print(format % args, flush=True)

        def is_loopback_request(self):
            host = self.headers.get("host", "")
            return (
                self.client_address[0] == LOOPBACK_HOST
                and host in (LOOPBACK_HOST, "{}:{}".format(LOOPBACK_HOST, LOOPBACK_PORT))
            )

        def reject_external(self):
            mark("DEN_TEST_EXTERNAL_MARKER")
            self.send_error(403, "only the loopback fixture is allowed")

        def do_CONNECT(self):
            self.reject_external()

        def do_GET(self):
            self.reject_external()

        def do_HEAD(self):
            if not self.is_loopback_request() or self.path != "/":
                self.reject_external()
                return
            self.send_response(204)
            self.end_headers()

        def do_POST(self):
            if not self.is_loopback_request() or not self.path.startswith("/v1/messages"):
                self.reject_external()
                return
            try:
                length = int(self.headers["content-length"])
                request = json.loads(self.rfile.read(length))
                content, stop_reason = response_content(request)
                if request.get("stream"):
                    encoded = stream_events(request, content, stop_reason)
                    content_type = "text/event-stream"
                else:
                    encoded = json.dumps(message(request, content, stop_reason)).encode()
                    content_type = "application/json"
            except Exception as error:
                mark("DEN_TEST_PROTOCOL_MARKER")
                self.send_error(400, str(error))
                return

            self.send_response(200)
            self.send_header("content-type", content_type)
            self.send_header("content-length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

    server = HTTPServer((LOOPBACK_HOST, LOOPBACK_PORT), Handler)
    mark("DEN_TEST_READY_MARKER")
    server.serve_forever()
    PYTHON

    python fixture.py > "$root/fixture.log" 2>&1 &
    fixturePID=$!
    trap 'kill "$fixturePID" 2>/dev/null || true' EXIT
    for _ in $(seq 1 100); do
      test -e "$DEN_TEST_READY_MARKER" && break
      sleep 0.05
    done
    test -e "$DEN_TEST_READY_MARKER"
    test "$settingsFingerprint" = "$(sha256sum "$settings" "$config/settings.json")"

    if ! timeout 30 ${pkgs.claude-code}/bin/claude \
      -p "run the fixture command" \
      ${mandatoryArgs} \
      > "$root/claude.out" 2> "$root/claude.err"; then
      cat "$root/claude.out" >&2
      cat "$root/claude.err" >&2
      cat "$root/fixture.log" >&2
      exit 1
    fi

    test -e "$DEN_TEST_TOOL_MARKER"
    test -e "$DEN_TEST_FENCE_MARKER"
    test -e "$DEN_TEST_USER_MARKER"
    test -e "$DEN_TEST_BASH_MARKER"
    test -e "$DEN_TEST_FINAL_MARKER"
    test ! -e "$DEN_TEST_EXTERNAL_MARKER"
    test ! -e "$DEN_TEST_PROTOCOL_MARKER"
    grep -Fx "fixture complete" "$root/claude.out"
    test "$(grep -c 'POST /v1/messages' "$root/fixture.log")" -eq 2
    touch "$out"
  ''
