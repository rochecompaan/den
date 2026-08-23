#!/usr/bin/env python3
import json
import subprocess
import sys
import unittest


SAFE_RUNTIME_LITERAL = {
    "derivations": {
        "fixture.drv": {"env": {"builderScript": "exec /bin/ls -lde /tmp/state"}}
    },
    "version": 3,
}

DIRECT_STRING = {
    "derivations": {
        "fixture.drv": {"env": {"__impureHostDeps": "/bin/sh /bin/ls /dev/null"}}
    },
    "version": 3,
}

NESTED_LIST = {
    "derivations": {
        "parent.drv": {"inputDrvs": {"child.drv": {"outputs": ["out"]}}},
        "child.drv": {"structuredAttrs": {"__impureHostDeps": ["/bin/sh", "/bin/ls"]}},
    },
    "version": 3,
}


class DerivationImpureHostDepsTests(unittest.TestCase):
    def run_parser(self, document):
        return subprocess.run(
            [sys.executable, PARSER],
            input=document if isinstance(document, str) else json.dumps(document),
            text=True,
            capture_output=True,
            check=False,
        )

    def test_allows_unrelated_runtime_literal(self):
        result = self.run_parser(SAFE_RUNTIME_LITERAL)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_allows_safe_dependency_list(self):
        result = self.run_parser(
            {
                "derivations": {
                    "fixture.drv": {"env": {"__impureHostDeps": ["/bin/sh"]}}
                },
                "version": 3,
            }
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_direct_string_dependency(self):
        result = self.run_parser(DIRECT_STRING)
        self.assertEqual(result.returncode, 1)
        self.assertIn(
            "derivations.fixture.drv.env.__impureHostDeps", result.stderr
        )

    def test_rejects_nested_list_dependency(self):
        result = self.run_parser(NESTED_LIST)
        self.assertEqual(result.returncode, 1)
        self.assertIn(
            "derivations.child.drv.structuredAttrs.__impureHostDeps", result.stderr
        )

    def test_rejects_propagated_dependency(self):
        result = self.run_parser(
            {
                "derivations": {
                    "fixture.drv": {
                        "env": {"__propagatedImpureHostDeps": "/bin/ls"}
                    }
                },
                "version": 3,
            }
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn(
            "derivations.fixture.drv.env.__propagatedImpureHostDeps", result.stderr
        )

    def test_rejects_malformed_json(self):
        result = self.run_parser("{")
        self.assertEqual(result.returncode, 2)
        self.assertIn("invalid derivation graph:", result.stderr)

    def test_rejects_non_string_field_value(self):
        result = self.run_parser(
            {
                "derivations": {
                    "fixture.drv": {"env": {"__impureHostDeps": {"bad": "value"}}}
                },
                "version": 3,
            }
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("impure host dependency field must be a string or string list", result.stderr)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit(f"usage: {sys.argv[0]} <parser>")
    PARSER = sys.argv.pop()
    unittest.main()
