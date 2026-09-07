{ pkgs }:

{ resources, extraPkgs ? [ ] }:
let
  lib = pkgs.lib;
  classes = [
    { name = "extensions"; flag = "--extension"; kind = "extension"; entries = resources.extensions; }
    { name = "packages"; flag = "--extension"; kind = "package"; entries = resources.packages; }
    { name = "skills"; flag = "--skill"; kind = "skill"; entries = resources.skills; }
    { name = "promptTemplates"; flag = "--prompt-template"; kind = "prompt template"; entries = resources.promptTemplates; }
    { name = "themes"; flag = "--theme"; kind = "theme"; entries = resources.themes; }
  ];
  argumentsFor = resource: lib.concatMap (entry: [ resource.flag "${entry}" ]) resource.entries;
  resourceArgs = lib.concatMap argumentsFor classes;
  diagnosticInputs = lib.concatMap (resource: map (entry: "${resource.kind}:${entry}") resource.entries) classes;
  extraInputs = map (entry: "${entry}/bin/pi") extraPkgs;
  diagnosticsCheck = pkgs.runCommand "pi-resource-validation"
    { nativeBuildInputs = [ pkgs.coreutils pkgs.findutils pkgs.gnugrep pkgs.jq ]; }
    ''
      set -euo pipefail
      declare -A seen
      validate() {
        kind=$1
        entry=$2
        canonical="$(${pkgs.coreutils}/bin/realpath -e -- "$entry")" || {
          echo "Pi $kind resource is missing: $entry" >&2
          exit 1
        }
        key="$kind:$canonical"
        if [ -n "''${seen[$key]+x}" ]; then
          echo "Pi $kind resources contain duplicate canonical path: $canonical" >&2
          exit 1
        fi
        seen[$key]=1
        case "$kind" in
          extension)
            if [ -f "$canonical" ]; then
              case "$canonical" in *.ts|*.js) ;; *) exit 1 ;; esac
            elif [ -d "$canonical" ]; then
              { [ -f "$canonical/index.ts" ] || [ -f "$canonical/index.js" ] || ${pkgs.findutils}/bin/find "$canonical" -maxdepth 2 -type f \( -name '*.ts' -o -name '*.js' \) -print -quit | ${pkgs.gnugrep}/bin/grep -q .; } || exit 1
            else
              exit 1
            fi
            ;;
          skill)
            { [ -f "$canonical" ] && [ "$(basename "$canonical")" = SKILL.md ]; } || { [ -d "$canonical" ] && ${pkgs.findutils}/bin/find "$canonical" -type f -name SKILL.md -print -quit | ${pkgs.gnugrep}/bin/grep -q .; } || exit 1
            ;;
          'prompt template')
            { [ -f "$canonical" ] && case "$canonical" in *.md) true ;; *) false ;; esac; } || { [ -d "$canonical" ] && ${pkgs.findutils}/bin/find "$canonical" -type f -name '*.md' -print -quit | ${pkgs.gnugrep}/bin/grep -q .; } || exit 1
            ;;
          theme)
            { [ -f "$canonical" ] && case "$canonical" in *.json) true ;; *) false ;; esac; } || { [ -d "$canonical" ] && ${pkgs.findutils}/bin/find "$canonical" -type f -name '*.json' -print -quit | ${pkgs.gnugrep}/bin/grep -q .; } || exit 1
            ;;
          package)
            [ -d "$canonical" ] || exit 1
            convention=false
            for directory in extensions skills prompts themes; do
              [ -d "$canonical/$directory" ] && convention=true
            done
            manifest=false
            if [ -f "$canonical/package.json" ]; then
              ${pkgs.jq}/bin/jq -e '(.pi | type) == "object"' "$canonical/package.json" >/dev/null && manifest=true
              while IFS= read -r dependency; do
                [ -f "$canonical/node_modules/$dependency/package.json" ] || {
                  echo "Pi package dependency is absent from the store closure: $dependency" >&2
                  exit 1
                }
              done < <(${pkgs.jq}/bin/jq -r '.dependencies // {} | keys[]' "$canonical/package.json")
            fi
            if [ "$manifest" != true ] && [ "$convention" != true ]; then
              echo "Pi package has neither Pi metadata nor Pi convention directories: $canonical" >&2
              exit 1
            fi
            ;;
        esac
      }
      ${lib.concatMapStringsSep "\n" (item: let parts = lib.splitString ":" item; in "validate ${lib.escapeShellArg (builtins.elemAt parts 0)} ${lib.escapeShellArg (lib.concatStringsSep ":" (lib.drop 1 parts))}") diagnosticInputs}
      ${lib.concatMapStringsSep "\n" (entry: "[ ! -e ${lib.escapeShellArg entry} ] || { echo 'extraPkgs must not expose bin/pi' >&2; exit 1; }") extraInputs}
      printf '%s\n' 'Pi resource validation passed; configured resource order remains adapter-owned and Pi retains same-name first-winner diagnostics.' > "$out"
    '';
in
{
  inherit resourceArgs diagnosticsCheck;
  closureInputs = [ diagnosticsCheck ];
}
