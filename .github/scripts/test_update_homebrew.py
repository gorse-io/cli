import base64
import unittest
from unittest.mock import patch

import update_homebrew as updater

from update_homebrew import update_formula


class UpdateFormulaTest(unittest.TestCase):
    def setUp(self):
        self.hashes = {
            "darwin_arm64": "a" * 64,
            "linux_amd64": "b" * 64,
            "linux_arm64": "c" * 64,
        }
        self.formula = 'class GorseCli < Formula\n  version "0.5.12"\n'
        for target in self.hashes:
            tag = "v0.5.12" if target == "darwin_arm64" else "v0.5.11"
            digest = self.hashes[target] if target == "darwin_arm64" else "0" * 64
            self.formula += (
                f'  url "https://github.com/gorse-io/cli/releases/download/{tag}/gorse-cli_{target}",\n'
                f'      using: :nounzip\n  sha256 "{digest}"\n'
            )
        self.formula += "end\n"

    def test_updates_both_linux_targets_and_preserves_macos(self):
        updated = update_formula(self.formula, "v0.5.12", self.hashes)
        self.assertNotIn("v0.5.11", updated)
        for target, digest in self.hashes.items():
            self.assertIn(f'/v0.5.12/gorse-cli_{target}', updated)
            self.assertIn(f'sha256 "{digest}"', updated)
        self.assertEqual(update_formula(updated, "v0.5.12", self.hashes), updated)

    def test_rejects_missing_target(self):
        with self.assertRaises(ValueError):
            update_formula(self.formula.replace("linux_arm64", "linux_riscv64"), "v0.5.12", self.hashes)

    def test_rejects_duplicate_target(self):
        with self.assertRaises(ValueError):
            update_formula(self.formula.replace("linux_arm64", "linux_amd64"), "v0.5.12", self.hashes)

    def test_requires_action_to_have_updated_macos_first(self):
        with self.assertRaises(ValueError):
            update_formula(self.formula.replace("v0.5.12", "v0.5.11"), "v0.5.12", self.hashes)

    def test_rejects_incorrect_macos_hash(self):
        with self.assertRaises(ValueError):
            update_formula(self.formula.replace("a" * 64, "0" * 64), "v0.5.12", self.hashes)

    def test_rejects_incorrect_formula_version(self):
        with self.assertRaises(ValueError):
            update_formula(self.formula.replace('version "0.5.12"', 'version "0.5.11"'), "v0.5.12", self.hashes)


    def test_completes_the_matching_pr_and_reads_back_the_write(self):
        state = {"content": self.formula, "writes": 0, "reads": 0}
        branch = "update-gorse-cli.rb-123"

        def api(endpoint, *, method="GET", payload=None, paginate=False):
            if endpoint.endswith("pulls?state=open&per_page=100"):
                self.assertTrue(paginate)
                return [[{"title": "unrelated", "head": {}}], [{
                    "title": "gorse-cli 0.5.12",
                    "head": {"ref": branch, "repo": {"full_name": "gorse-io/homebrew-tap"}},
                }]]
            self.assertIn("contents/Formula/gorse-cli.rb", endpoint)
            if method == "PUT":
                assert payload is not None
                self.assertEqual(payload["branch"], branch)
                self.assertEqual(payload["sha"], "original-blob")
                state["content"] = base64.b64decode(payload["content"]).decode()
                state["writes"] += 1
                return {}
            self.assertIn(f"ref={branch}", endpoint)
            state["reads"] += 1
            return {"sha": "original-blob", "content": base64.b64encode(state["content"].encode()).decode()}

        with patch.object(updater, "gh_api", api, create=True), patch.object(
            updater, "checksum", lambda tag, target: self.hashes[target], create=True
        ):
            updater.complete_update("v0.5.12")
        self.assertEqual(state["writes"], 1)
        self.assertEqual(state["reads"], 2)
        self.assertNotIn("v0.5.11", state["content"])

    def test_rejects_ambiguous_prs_before_any_write(self):
        pr = {"title": "gorse-cli 0.5.12", "head": {
            "ref": "update-gorse-cli.rb-123", "repo": {"full_name": "gorse-io/homebrew-tap"},
        }}
        with patch.object(updater, "gh_api", return_value=[[pr, pr]], create=True) as api:
            with self.assertRaises(ValueError):
                updater.complete_update("v0.5.12")
            self.assertEqual(api.call_count, 1)


if __name__ == "__main__":
    unittest.main()
