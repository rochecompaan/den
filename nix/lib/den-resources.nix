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
  force = values: lib.foldl' (result: value: builtins.seq value result) true values;
  isResource = value: lib.isDerivation value || builtins.isPath value;
  isSettingsFragment = value: isResource value || (builtins.isAttrs value && !lib.isDerivation value);

  validateContribution = bundle: agentName: contribution:
    let
      bundleLabel = "Den bundle ${bundleName bundle}";
      classes = agentClasses.${agentName};
      validateList = class:
        let
          entries = contribution.${class};
          isEntry = if agentName == "claude" && class == "settings"
            then isSettingsFragment
            else isResource;
        in
        assert lib.assertMsg (builtins.isList entries)
          "${bundleLabel} ${agentName}.${class} must be a list";
        assert lib.assertMsg (lib.all isEntry entries)
          "${bundleLabel} ${agentName}.${class} must contain only Nix paths or packages";
        entries;
      validateAttrs = class:
        let entries = contribution.${class}; in
        assert lib.assertMsg (builtins.isAttrs entries)
          "${bundleLabel} ${agentName}.${class} must be an attribute set";
        assert lib.assertMsg (lib.all builtins.isAttrs (builtins.attrValues entries))
          "${bundleLabel} ${agentName}.${class} must be an attribute set of server definitions";
        entries;
    in
    assert lib.assertMsg (builtins.isAttrs contribution)
      "${bundleLabel} ${agentName} resources must be an attribute set";
    assert lib.assertMsg (hasOnly (classes.listClasses ++ classes.attrClasses) contribution)
      "${bundleLabel} declares an unknown resource class";
    assert force
      (map validateList (builtins.filter (class: builtins.hasAttr class contribution) classes.listClasses)
        ++ map validateAttrs (builtins.filter (class: builtins.hasAttr class contribution) classes.attrClasses));
    contribution;

  validateBundle = bundle:
    assert lib.assertMsg (lib.isDerivation bundle)
      "Den bundle must be a package";
    assert lib.assertMsg (bundle ? denResources && builtins.isAttrs bundle.denResources)
      "Den bundle is missing passthru.denResources: ${bundleName bundle}";
    assert lib.assertMsg (hasOnly knownAgents bundle.denResources)
      "Den bundle declares an unknown agent: ${bundleName bundle}";
    assert force (map (agentName: validateContribution bundle agentName bundle.denResources.${agentName})
      (builtins.attrNames bundle.denResources));
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
