#!/usr/bin/env python3
import json
import shlex
import sys

FORBIDDEN = "/bin/ls"
IMPURE_FIELDS = {"__impureHostDeps", "__propagatedImpureHostDeps"}


def dependency_tokens(value):
    if isinstance(value, str):
        return shlex.split(value)
    if isinstance(value, list) and all(isinstance(item, str) for item in value):
        return value
    raise ValueError("impure host dependency field must be a string or string list")


def forbidden_paths(value, path=()):
    matches = []
    if isinstance(value, dict):
        for key, child in value.items():
            child_path = path + (key,)
            if key in IMPURE_FIELDS and FORBIDDEN in dependency_tokens(child):
                matches.append(child_path)
            matches.extend(forbidden_paths(child, child_path))
    elif isinstance(value, list):
        for index, child in enumerate(value):
            matches.extend(forbidden_paths(child, path + (str(index),)))
    return matches


def main():
    document = json.load(sys.stdin)
    matches = forbidden_paths(document)
    for path in matches:
        print(
            f"forbidden impure host dependency {FORBIDDEN}: {'.'.join(path)}",
            file=sys.stderr,
        )
    return 1 if matches else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (json.JSONDecodeError, ValueError) as error:
        print(f"invalid derivation graph: {error}", file=sys.stderr)
        raise SystemExit(2)
