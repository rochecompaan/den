{ lib }:

let
  # A string such as "${pkg}/skills" names immutable store content only while it
  # keeps the string context that pulls its package into the closure.
  isStorePathString = value:
    builtins.isString value
    && lib.hasPrefix "${builtins.storeDir}/" value
    && builtins.hasContext value
    && !(builtins.elem ".." (lib.splitString "/" value));
in
{
  isResource = value: builtins.isPath value || lib.isDerivation value || isStorePathString value;
}
