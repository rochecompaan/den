{ lib }:

rawNetwork:
let
  unknownFields = lib.subtractLists [ "extraAllowedDomains" ] (builtins.attrNames rawNetwork);
  domains = rawNetwork.extraAllowedDomains or [ ];
  isLabel = label:
    builtins.stringLength label <= 63
    && builtins.match "[a-z0-9]([a-z0-9-]*[a-z0-9])?" label != null;
  isHostname = hostname:
    let
      labels = lib.splitString "." hostname;
    in
    builtins.stringLength hostname <= 253
    && builtins.length labels >= 2
    && lib.all isLabel labels
    && builtins.match ".*[a-z].*" (lib.last labels) != null;
  isDomain = value:
    builtins.isString value && isHostname (lib.removePrefix "*." value);
in
assert lib.assertMsg (builtins.isAttrs rawNetwork) "network must be an attribute set";
assert lib.assertMsg (unknownFields == [ ])
  "network has unknown fields: ${builtins.toJSON unknownFields}";
assert lib.assertMsg (builtins.isList domains) "network.extraAllowedDomains must be a list of strings";
{
  extraAllowedDomains = lib.imap0 (index: value:
    assert lib.assertMsg (isDomain value)
      "network.extraAllowedDomains[${toString index}] must be a lowercase hostname or a leading *. pattern";
    value
  ) domains;
}
