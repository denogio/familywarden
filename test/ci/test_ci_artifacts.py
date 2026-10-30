"""Fast provenance and fetch-gate regression tests; no network or SDK required."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("ci_artifacts", ROOT / "scripts/ci-artifacts.py")
artifacts = importlib.util.module_from_spec(spec)
spec.loader.exec_module(artifacts)
SHA = "a" * 40
ENV = {"GITHUB_SHA": SHA, "GITHUB_REPOSITORY": "denogio/familywarden",
       "GITHUB_RUN_ID": "42", "GITHUB_RUN_ATTEMPT": "2", "GITHUB_RUN_NUMBER": "100",
       "MAST_RELEASE_SIGNED": "true"}


class ArtifactTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.versions = artifacts.package_versions()
        self.android = self.versions["familywarden"].rsplit("-r", 1)[0]
        for group in artifacts.GROUPS:
            folder = self.directory / f"{group}-{SHA}"
            folder.mkdir()
            for name in artifacts.expected_files(group, self.versions, self.android):
                (folder / name).write_bytes(b"synthetic package")
            with patch.dict(os.environ, ENV):
                artifacts.create(group, folder)

    def verify(self):
        artifacts.verify(self.directory, SHA, ENV["GITHUB_REPOSITORY"], "42", "2")

    def modify(self, group, key, value):
        path = self.directory / f"{group}-{SHA}" / "provenance.json"
        data = json.loads(path.read_text())
        data[key] = value
        path.write_text(json.dumps(data))

    def test_valid_manifest_round_trip(self):
        self.verify()

    def test_rejects_tampered_package(self):
        next((self.directory / f"apks-{SHA}").glob("*.apk")).write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            self.verify()

    def test_rejects_wrong_sha_repository_run_or_attempt(self):
        for key in ("sha", "repository", "run_id", "run_attempt"):
            with self.subTest(key=key):
                self.modify("apks", key, "wrong")
                with self.assertRaisesRegex(ValueError, f"mismatched {key}"):
                    self.verify()
                self.modify("apks", key, {"sha": SHA, "repository": ENV["GITHUB_REPOSITORY"],
                                            "run_id": "42", "run_attempt": "2"}[key])

    def test_accepts_earlier_attempt_of_same_run_but_not_later(self):
        self.modify("apks", "run_attempt", "1")
        self.verify()
        self.modify("apks", "run_attempt", "3")
        with self.assertRaisesRegex(ValueError, "mismatched run_attempt"):
            self.verify()

    def test_rejects_extra_package(self):
        (self.directory / f"apks-{SHA}" / "unexpected.apk").touch()
        with self.assertRaisesRegex(ValueError, "unexpected or missing"):
            self.verify()

    def test_rejects_unsigned_android_release(self):
        self.modify("mast", "android_release_signed", False)
        with self.assertRaisesRegex(ValueError, "not signed"):
            self.verify()

    def test_rejects_inconsistent_package_versions(self):
        versions = copy.copy(self.versions)
        versions["luci-app-familywarden"] = "wrong"
        self.modify("core-aarch64", "package_versions", versions)
        with self.assertRaisesRegex(ValueError, "Inconsistent package_versions"):
            self.verify()

    def test_generation_rejects_missing_package(self):
        folder = self.directory / f"apks-{SHA}"
        next(folder.glob("*.apk")).unlink()
        with patch.dict(os.environ, ENV), self.assertRaisesRegex(ValueError, "Unexpected apks"):
            artifacts.create("apks", folder)

    def fetch_green_run(self, runs):
        fake_bin = self.directory / "bin"
        fake_bin.mkdir()
        gh = fake_bin / "gh"
        gh.write_text('''#!/bin/bash
case "$1 $2" in
  'repo view') echo denogio/familywarden ;;
  'run list') echo "$RUN_LIST" ;;
  'api repos/denogio/familywarden/actions/runs/42') echo "$RUN_JSON" ;;
  'api --paginate') echo success ;;
  'run download') cp -R "$FIXTURE_SOURCE/$7" "$9" ;;
  *) exit 99 ;;
esac
''')
        gh.chmod(0o755)
        run = {"head_sha": SHA, "status": "completed", "conclusion": "success", "run_attempt": 2}
        env = {**os.environ, "PATH": f"{fake_bin}:{os.environ['PATH']}",
               "RUN_JSON": json.dumps(run), "RUN_LIST": json.dumps(runs),
               "FIXTURE_SOURCE": str(self.directory)}
        result = subprocess.run(["bash", str(ROOT / "scripts/fetch-ci-artifacts.sh"), SHA,
                                 str(self.directory / "fetched")], env=env,
                                text=True, capture_output=True)
        return result

    def test_fetch_downloads_and_verifies_green_run(self):
        result = self.fetch_green_run([{"databaseId": 42, "event": "push"}])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Verified " + SHA, result.stdout)
        self.assertIn("/runs/42/attempts/2", result.stdout)

    def test_fetch_skips_newer_pr_run(self):
        result = self.fetch_green_run([{"databaseId": 43, "event": "pull_request"},
                                      {"databaseId": 42, "event": "push"}])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("/runs/42/attempts/2", result.stdout)

    def test_fetch_refuses_pr_only_history(self):
        result = self.fetch_green_run([{"databaseId": 43, "event": "pull_request"}])
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("No non-PR CI run", result.stderr)

    def test_fetch_refuses_failed_running_wrong_revision_or_failed_gate(self):
        fake_bin = self.directory / "bin"
        fake_bin.mkdir()
        gh = fake_bin / "gh"
        gh.write_text('''#!/bin/bash
case "$1 $2" in
  'repo view') echo denogio/familywarden ;;
  'run list') echo "$RUN_LIST" ;;
  'api repos/denogio/familywarden/actions/runs/42') echo "$RUN_JSON" ;;
  'api --paginate') echo "${GATE_RESULT:-failure}" ;;
  *) echo 'Unexpected download or command' >&2; exit 99 ;;
esac
''')
        gh.chmod(0o755)
        good = {"head_sha": SHA, "status": "completed", "conclusion": "success", "run_attempt": 2}
        cases = [{**good, "conclusion": "failure"}, {**good, "status": "in_progress"},
                 {**good, "head_sha": "b" * 40}, good]
        for number, run in enumerate(cases):
            env = {**os.environ, "PATH": f"{fake_bin}:{os.environ['PATH']}", "RUN_JSON": json.dumps(run),
                   "RUN_LIST": json.dumps([{"databaseId": 42, "event": "push"}])}
            result = subprocess.run(["bash", str(ROOT / "scripts/fetch-ci-artifacts.sh"), SHA,
                                     str(self.directory / f"output-{number}")], env=env,
                                    text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn("Unexpected download", result.stderr)


class WorkflowGateTests(unittest.TestCase):
    def test_concurrency_isolates_manual_dispatch_from_push(self):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        concurrency = workflow.split("concurrency:\n", 1)[1].split("\npermissions:", 1)[0]
        self.assertIn("group: ci-${{ github.workflow }}-${{ github.event_name }}-${{ github.ref }}", concurrency)
        self.assertIn("cancel-in-progress: ${{ github.event_name == 'push' && !startsWith(github.ref, 'refs/tags/') }}", concurrency)

    def gate(self, results, requested=False):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        section = workflow.split("  gate:\n", 1)[1].split("  release:\n", 1)[0]
        code = textwrap.dedent(section.split("python3 - <<'PY'\n", 1)[1].split("          PY\n", 1)[0])
        env = {**os.environ, "RESULTS": json.dumps({key: {"result": value} for key, value in results.items()}),
               "EMULATOR_REQUIRED": str(requested).lower()}
        return subprocess.run(["python3", "-c", code], env=env, capture_output=True).returncode

    def test_gate_requires_every_mandatory_job(self):
        results = {job: "success" for job in ("test", "core", "build", "core-aarch64", "e2e", "mast")}
        results["mast-emulator"] = "skipped"
        self.assertEqual(self.gate(results), 0)
        for job in results:
            if job == "mast-emulator":
                continue
            for state in ("failure", "cancelled", "skipped"):
                with self.subTest(job=job, state=state):
                    self.assertNotEqual(self.gate({**results, job: state}), 0)

    def test_gate_requires_requested_emulator(self):
        results = {job: "success" for job in ("test", "core", "build", "core-aarch64", "e2e", "mast")}
        for state in ("failure", "cancelled", "skipped"):
            self.assertNotEqual(self.gate({**results, "mast-emulator": state}, requested=True), 0)
        self.assertEqual(self.gate({**results, "mast-emulator": "success"}, requested=True), 0)


if __name__ == "__main__":
    unittest.main()
