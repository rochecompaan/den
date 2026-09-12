{ pkgs }:

{ agent, bundles, resources }:
let
  lib = pkgs.lib;
  agentClasses = {
    pi = { listClasses = [ "extensions" "packages" "skills" "promptTemplates" "themes" ]; attrClasses = [ ]; };
    claude = { listClasses = [ "skills" "plugins" "settings" ]; attrClasses = [ "mcpServers" ]; };
  };
  knownAgents = builtins.attrNames agentClasses;
  bundleName = bundle: bundle.name or "<unnamed>";
  hasOnly = allowed: value: lib.all (name: builtins.elem name allowed) (builtins.attrNames value);

  validateBundle = bundle:
    assert lib.assertMsg (lib.isDerivation bundle)
      "Den bundle must be a package";
    assert lib.assertMsg (bundle ? denResources && builtins.isAttrs bundle.denResources)
      "Den bundle is missing passthru.denResources: ${bundleName bundle}";
    assert lib.assertMsg (hasOnly knownAgents bundle.denResources)
      "Den bundle declares an unknown agent: ${bundleName bundle}";
    assert lib.assertMsg (lib.all
      (agentName:
        hasOnly (agentClasses.${agentName}.listClasses ++ agentClasses.${agentName}.attrClasses)
          bundle.denResources.${agentName})
      (builtins.attrNames bundle.denResources))
      "Den bundle declares an unknown resource class: ${bundleName bundle}";
    bundle;

  validated = map validateBundle bundles;
  contribution = bundle: bundle.denResources.${agent} or { };
  classes = agentClasses.${agent};

  mergedList = class:
    lib.concatMap (bundle: (contribution bundle).${class} or [ ]) validated
    ++ resources.${class};

  mergedAttrs = class:
    lib.foldl'
      (accumulated: additions:
        assert lib.assertMsg
          (builtins.intersectAttrs accumulated additions == { })
          "Den bundles declare a duplicate ${class} name";
        accumulated // additions)
      { }
      (map (bundle: (contribution bundle).${class} or { }) validated
        ++ [ resources.${class} ]);

  bundleValidation = lib.foldl' (_: bundle: builtins.seq bundle true) true validated;
  attrValidation = lib.foldl' (_: class: builtins.seq (mergedAttrs class) true) true classes.attrClasses;
in
builtins.seq bundleValidation (builtins.seq attrValidation (
  lib.genAttrs classes.listClasses mergedList
  // lib.genAttrs classes.attrClasses mergedAttrs
))
