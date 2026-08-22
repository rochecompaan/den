{ inputs, pkgs, claude }:

let
  fence = (import ../lib/fence.nix { inherit pkgs; }).package;
  repoWolfClient = import ../packages/repowolf-client.nix { inherit inputs pkgs; };
  repoWolfFixture = pkgs.buildGoModule {
    pname = "den-native-repowolf-fixture";
    version = "${inputs.repowolf.shortRev or "pinned"}";
    src = inputs.repowolf;
    vendorHash = "sha256-gTntGkqO04KwcyrJi3jNVwNAevKQddTGU3npLupIWik=";
    postPatch = ''
      mkdir -p cmd/den-native-repowolf-fixture
      cp ${./repowolf-protocol-fixture.go} cmd/den-native-repowolf-fixture/main.go
    '';
    subPackages = [ "cmd/den-native-repowolf-fixture" ];
    tags = [ "repowolf_native_fixture" ];
    env.CGO_ENABLED = "0";
  };
  fixtureAgent = pkgs.writeShellApplication {
    name = "den-native-agent";
    runtimeInputs = [ pkgs.bash pkgs.coreutils pkgs.curl pkgs.gitMinimal pkgs.gnugrep ];
    text = ''
      set -eu
      scenario="''${1:?native fixture scenario is required}"
      shift
      case "$scenario" in
        network-allow)
          exec curl --fail --silent --show-error --cacert "$REPOWOLF_CA_FILE" "$@"
          ;;
        network-deny)
          if curl --fail --silent --show-error --connect-timeout 2 \
            --cacert "$REPOWOLF_CA_FILE" --resolve "$1:$2:127.0.0.1" "https://$1:$2/"; then
            echo "denied host was reachable" >&2
            exit 1
          fi
          ;;
        credential-deny)
          if cat "$1"; then echo "credential link was readable" >&2; exit 1; fi
          ;;
        write-deny)
          if mkdir -p "$1" 2>/dev/null && printf denied > "$1/probe" 2>/dev/null; then
            echo "denied path was writable" >&2
            exit 1
          fi
          ;;
        write-file-deny)
          if printf denied > "$1" 2>/dev/null; then
            echo "denied file was writable" >&2
            exit 1
          fi
          ;;
        policy-immutable)
          replacement="$DEN_FENCE_TMPDIR/replacement"
          printf replacement > "$replacement"
          ! sh -c ': > "$1"' den "$DEN_FENCE_POLICY_FILE"
          ! cp "$replacement" "$DEN_FENCE_POLICY_FILE"
          ! mv "$DEN_FENCE_POLICY_FILE" "$DEN_FENCE_POLICY_FILE.old"
          ! chmod u+w "$DEN_FENCE_POLICY_FILE"
          ! sh -c 'printf mutation >> "$1"' den "$DEN_FENCE_POLICY_FILE"
          ;;
        effective-deny)
          ! cat "$1"
          ;;
        implicit-host-linux)
          sh -c 'printf mutation > "$1"' den "$1" 2>/dev/null || true
          sh -c 'printf child > "$1"' den "$2" 2>/dev/null || true
          ;;
        implicit-host-darwin)
          for path in "$@"; do
            if sh -c 'printf mutation > "$1"' den "$path" 2>/dev/null; then
              echo "implicit host temporary path was writable" >&2
              exit 1
            fi
          done
          ;;
        ca-read-only)
          grep -q 'BEGIN CERTIFICATE' "$REPOWOLF_CA_FILE"
          ! sh -c 'printf mutation >> "$1"' den "$REPOWOLF_CA_FILE"
          ;;
        argv-deny)
          exec git reset --hard
          ;;
        plugin-mcp)
          plugin="" mcp=""
          while test "$#" -gt 0; do
            case "$1" in
              --plugin-dir) plugin=$2; shift 2 ;;
              --mcp-config) mcp=$2; shift 2 ;;
              --strict-mcp-config) shift ;;
              *) shift ;;
            esac
          done
          test -n "$plugin" && test -n "$mcp"
          bash "$plugin/probe.sh"
          bash "$mcp"
          ;;
        repowolf)
          gh issue list --repo alpha/repo >/dev/null 2>&1 || true
          exec git ls-remote git@github.com:alpha/repo.git
          ;;
        marker)
          printf started > "$1"
          ;;
        *) echo "unknown native fixture scenario" >&2; exit 2 ;;
      esac
    '';
  };
  mkAgentSandbox = import ../lib/mk-agent-sandbox.nix { inherit inputs pkgs; };
  launcher = import ../packages/den-launcher.nix { inherit pkgs; };
  fixtureSandbox = mkAgentSandbox {
    configDir = null;
    extraPkgs = [ ];
    docker = {
      enable = true;
      package = fixtureAgent;
      composePackage = fixtureAgent;
      socketPath = "/tmp/den-native-container.sock";
      hostPorts = [ 38413 38414 ];
    };
    podman = { };
    adapter = {
      runtimePackages = [ fixtureAgent ];
      agent = {
        name = "native-fixture";
        executable = "${fixtureAgent}/bin/den-native-agent";
        mandatoryArgs = [ ];
        reservedFlags = [ ];
        configEnvironment = "CLAUDE_CONFIG_DIR";
        darwinSettings = "";
      };
    };
  };
  unrelatedStoreFile = pkgs.writeText "den-native-unrelated" "must remain unreadable\n";
  nativeTests = pkgs.buildGoModule {
    pname = "den-native-tests";
    version = "0.1.0";
    src = ../..;
    vendorHash = null;
    env.CGO_ENABLED = "0";
    doCheck = false;
    buildPhase = ''
      runHook preBuild
      go test -c -tags=native -o den-native-tests ./tests/native
      runHook postBuild
    '';
    installPhase = ''
      runHook preInstall
      install -Dm755 den-native-tests "$out/bin/den-native-tests"
      runHook postInstall
    '';
  };
in
pkgs.writeShellApplication {
  name = "native-enforcement";
  runtimeInputs = [ pkgs.bash pkgs.coreutils pkgs.curl pkgs.gitMinimal pkgs.gnused ]
    ++ pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.acl pkgs.iproute2 pkgs.util-linux ];
  text = ''
    export DEN_NATIVE_HOST_SYSTEM=${pkgs.stdenv.hostPlatform.system}
    export DEN_NATIVE_TEST_BINARY=${nativeTests}/bin/den-native-tests
    export DEN_NATIVE_CLAUDE=${claude}/bin/claude
    export DEN_NATIVE_SANDBOX=${fixtureSandbox}/bin/claude
    export DEN_NATIVE_MANIFEST=${fixtureSandbox.denManifest}
    export DEN_NATIVE_LAUNCHER=${launcher}/bin/den-launcher
    export DEN_NATIVE_FENCE=${fence}/bin/fence
    export DEN_NATIVE_REPOWOLF_CLIENT_DIR=${repoWolfClient}
    export DEN_NATIVE_REPOWOLF_FIXTURE=${repoWolfFixture}/bin/den-native-repowolf-fixture
    export DEN_NATIVE_UNRELATED_STORE_FILE=${unrelatedStoreFile}
    ${pkgs.lib.optionalString pkgs.stdenv.isLinux ''
      export DEN_NATIVE_ACL=${pkgs.acl}/bin/setfacl
      export DEN_NATIVE_GETFACL=${pkgs.acl}/bin/getfacl
      export DEN_NATIVE_UNSHARE=${pkgs.util-linux}/bin/unshare
      export DEN_NATIVE_MOUNT=${pkgs.util-linux}/bin/mount
      export DEN_NATIVE_IP=${pkgs.iproute2}/bin/ip
      export DEN_NATIVE_BASH=${pkgs.bash}/bin/bash
    ''}
    ${builtins.readFile ./native-runner.sh}
  '';
}
