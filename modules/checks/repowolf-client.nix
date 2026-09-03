{ inputs, ... }:

{
  perSystem = { pkgs, ... }:
    let
      repowolfClient = import ../../nix/packages/repowolf-client.nix {
        inherit inputs pkgs;
      };
    in
    {
      checks.repowolf-client = repowolfClient;
      checks.repowolf-client-closure = import ../../nix/check-support/repowolf-client.nix {
        inherit pkgs repowolfClient;
      };
    };
}
