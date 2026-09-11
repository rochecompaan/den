{ inputs, pkgs }:

let
  lib = pkgs.lib;
  mkAgentSandbox = import ../lib/mk-agent-sandbox.nix { inherit inputs pkgs; };
  dependency = pkgs.writeShellScriptBin "dependency" "exit 0";
  adapter = {
    runtimePackages = [ ];
    closureOnlyPackages = [ ];
    output = {
      packageName = "test-agent";
      commandName = "test-agent";
      manifestName = "test-agent-manifest.json";
      mainProgram = "test-agent";
    };
    agent = {
      name = "test-agent";
      executable = "${dependency}/bin/dependency";
      argumentPolicy = "claude";
      mandatoryArgs = [ ];
      resourceArgs = [ ];
      reservedFlags = [ ];
      reservedCommands = [ ];
      environment = { scrub = [ ]; set = { }; };
      packageDirectory = null;
      securityAdapter = null;
    };
    stateBindings = [{
      name = "config";
      explicitPath = null;
      inheritedEnvironment = "TEST_AGENT_CONFIG_DIR";
      defaultPath = "";
      defaultWritablePaths = [ ];
      exports = [{ kind = "environment"; name = "TEST_AGENT_CONFIG_DIR"; exportDefault = false; }];
    }];
  };
  withOutput = output: mkAgentSandbox {
    adapter = adapter // { inherit output; };
    configDir = null;
    extraPkgs = [ ];
    docker = { };
    podman = { };
    dependencies = {
      fence = dependency;
      repoWolfClient = dependency;
      launcher = dependency;
      git = dependency;
      bash = dependency;
      coreutils = dependency;
      acl = dependency;
      aclProbeDarwin = dependency;
    };
  };
  result = withOutput adapter.output;
  literal = withOutput {
    packageName = "literal-agent";
    commandName = "literal$(echo altered)";
    manifestName = "literal-manifest.json";
    mainProgram = "literal-main";
  };
  # Force constructor acceptance, not store-path validation by Nix itself.
  rejects = field: value: !(builtins.tryEval
    (withOutput (adapter.output // { ${field} = value; })).name).success;
  invalidNames = field: [ "" "." ".." ] ++ map
    (value: value + lib.optionalString (field == "manifestName") ".json")
    [ "bad/name" "line\nfeed" "carriage\rreturn" ];
  rejectsUnsafeNames = field: lib.all (value:
    lib.assertMsg (rejects field value)
      "adapter accepted unsafe ${field}: ${builtins.toJSON value}"
  ) (invalidNames field);
in
assert lib.assertMsg (result.name == "test-agent")
  "adapter packageName ignored: expected test-agent, got ${result.name}";
assert result.meta.mainProgram == "test-agent";
assert literal.name == "literal-agent";
assert literal.meta.mainProgram == "literal-main";
assert lib.all rejectsUnsafeNames [ "packageName" "commandName" "manifestName" "mainProgram" ];
assert rejects "manifestName" "manifest.txt";
pkgs.runCommand "agent-adapter" { } ''
  set -eu
  test -x ${result}/bin/test-agent
  # The complete output file set excludes unrequested Claude wrappers/manifests.
  test "$(find ${result} -mindepth 1 ! -type d -printf '%P\n')" = bin/test-agent
  case "${result.denManifest}" in
    *-claude-manifest.json) exit 1 ;;
    *-test-agent-manifest.json) ;;
    *) exit 1 ;;
  esac
  test -f ${result.denManifest}
  test -x '${literal}/bin/literal$(echo altered)'
  test "$(find ${literal} -mindepth 1 ! -type d -printf '%P\n')" = 'bin/literal$(echo altered)'
  case "${literal.denManifest}" in *-literal-manifest.json) ;; *) exit 1 ;; esac
  test -f ${literal.denManifest}
  echo 'adapter output names, exact file sets, and literal shell metacharacters verified'
  touch "$out"
''
