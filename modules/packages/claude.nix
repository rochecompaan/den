{ ... }:
{
  perSystem = { self', ... }:
    let
      claude = self'.lib.mkClaude { };
    in
    {
      packages = {
        inherit claude;
        default = claude;
      };
    };
}
