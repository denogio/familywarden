#!/usr/bin/env python3
"""Record and verify CI package provenance; never deploy or handle signing keys."""
import hashlib
import json
import os
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
GROUPS = {
    "apks": ("familywarden", "familywarden-core", "luci-app-familywarden"),
    "core-aarch64": ("familywarden-core",),
    "mast": (),
}


def package_versions():
    versions = {}
    for package in GROUPS["apks"]:
        text = (ROOT / package / "Makefile").read_text()
        version = re.search(r"^PKG_VERSION:=(.+)$", text, re.M)[1]
        release = re.search(r"^PKG_RELEASE:=(.+)$", text, re.M)[1]
        versions[package] = f"{version}-r{release}"
    return versions


def expected_files(group, versions, android_version):
    if group == "mast":
        return {f"ulysses-mast-{android_version}.apk", "ulysses-mast-debug.apk"}
    return {f"{package}-{versions[package]}.apk" for package in GROUPS[group]}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def create(group, directory):
    versions = package_versions()
    android_version = versions["familywarden"].rsplit("-r", 1)[0]
    files = {p.name: digest(p) for p in directory.glob("*.apk")}
    if set(files) != expected_files(group, versions, android_version):
        raise ValueError(f"Unexpected {group} package set: {sorted(files)}")
    manifest = {
        "schema": 1, "group": group, "sha": os.environ["GITHUB_SHA"],
        "repository": os.environ["GITHUB_REPOSITORY"],
        "run_id": os.environ["GITHUB_RUN_ID"],
        "run_attempt": os.environ["GITHUB_RUN_ATTEMPT"],
        "package_versions": versions, "android_version_name": android_version,
        "android_version_code": os.environ["GITHUB_RUN_NUMBER"],
        "android_release_signed": os.environ.get("MAST_RELEASE_SIGNED") == "true",
        "validation": {
            "apks": "x86_64 install/upgrade smoke; LuCI e2e required by gate",
            "core-aarch64": "aarch64_cortex-a53 SDK build only; no install/hardware smoke",
            "mast": "unit tests, lint, release/debug builds; emulator only when requested",
        }[group],
        "files": files,
    }
    (directory / "provenance.json").write_text(json.dumps(manifest, indent=2) + "\n")


def verify(directory, sha, repository, run_id, attempt):
    manifests = []
    for group in GROUPS:
        folder = directory / f"{group}-{sha}"
        manifest = json.loads((folder / "provenance.json").read_text())
        for key, value in {"schema": 1, "group": group, "sha": sha,
                           "repository": repository, "run_id": run_id}.items():
            if manifest.get(key) != value:
                raise ValueError(f"{group}: mismatched {key}")
        # "Re-run failed jobs" keeps artifacts from earlier attempts of the same run.
        produced = str(manifest.get("run_attempt", ""))
        if not produced.isdigit() or not 1 <= int(produced) <= int(attempt):
            raise ValueError(f"{group}: mismatched run_attempt")
        files = manifest["files"]
        expected = expected_files(group, manifest["package_versions"], manifest["android_version_name"])
        if set(files) != expected or {p.name for p in folder.iterdir()} != expected | {"provenance.json"}:
            raise ValueError(f"{group}: unexpected or missing files")
        for name, checksum in files.items():
            if Path(name).name != name or digest(folder / name) != checksum:
                raise ValueError(f"{group}: checksum mismatch: {name}")
        manifests.append(manifest)
    if not manifests[-1]["android_release_signed"]:
        raise ValueError("Android release was not signed with the configured release key; not deployable")
    for manifest in manifests[1:]:
        for key in ("package_versions", "android_version_name", "android_version_code"):
            if manifest[key] != manifests[0][key]:
                raise ValueError(f"Inconsistent {key} across artifacts")
    print(f"Verified {sha}, run {run_id} attempt {attempt}: three artifact groups and all package SHA-256 hashes")
    print(json.dumps(manifests[0]["package_versions"], sort_keys=True))


def main():
    if sys.argv[1] == "create":
        create(sys.argv[2], Path(sys.argv[3]))
    elif sys.argv[1] == "verify":
        verify(Path(sys.argv[2]), *sys.argv[3:])
    else:
        raise ValueError("Expected create or verify")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError) as error:
        sys.exit(str(error))
