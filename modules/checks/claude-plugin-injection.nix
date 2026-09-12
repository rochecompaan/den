{ ... }:
{
  perSystem = { pkgs, ... }: {
    checks.claude-plugin-injection = pkgs.runCommand "claude-plugin-injection-check"
      {
        nativeBuildInputs = [
          (import ../../nix/check-support/claude-plugin-injection.nix { inherit pkgs; })
        ];
      }
      ''
        claude-plugin-injection
        touch "$out"
      '';
  };
}
