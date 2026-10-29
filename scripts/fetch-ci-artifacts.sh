#!/bin/bash
# Fetch only green, revision-bound CI packages. Uses the active gh account; never deploys.
set -euo pipefail
if [ "$#" -ne 2 ] || [[ ! "$1" =~ ^[0-9a-f]{40}$ ]]; then
    echo "Usage: $0 <full-commit-sha> <new-output-directory>" >&2
    exit 1
fi
sha="$1"
out="$2"
root="$(cd "$(dirname "$0")/.." && pwd)"
# A fresh directory prevents accidentally mixing runs or stale packages.
mkdir "$out"
repo="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
# PR artifacts belong to a synthetic merge SHA, not the requested source SHA.
run="$(gh run list --repo "$repo" --workflow ci.yml --commit "$sha" --limit 100 \
    --json databaseId,event | python3 -c '
import json, sys
runs = json.load(sys.stdin)
print(next((run["databaseId"] for run in runs if run["event"] != "pull_request"), ""))
')"
[ -n "$run" ] || { echo 'No non-PR CI run for this SHA in the latest 100 runs' >&2; exit 1; }
metadata="$(gh api "repos/$repo/actions/runs/$run")"
attempt="$(printf '%s' "$metadata" | python3 -c '
import json, sys
run = json.load(sys.stdin)
if run["head_sha"] != sys.argv[1] or run["status"] != "completed" or run["conclusion"] != "success":
    sys.exit("Latest non-PR CI run for this SHA is not completed and successful")
print(run["run_attempt"])
' "$sha")"
# Inspect the current attempt, not a successful gate from an older attempt.
gh api --paginate "repos/$repo/actions/runs/$run/attempts/$attempt/jobs?per_page=100" \
    --jq '.jobs[] | select(.name == "Validated artifacts gate") | .conclusion' \
    | grep -Fxq success || { echo 'Validated artifacts gate is not green' >&2; exit 1; }
for group in apks core-aarch64 mast; do
    gh run download "$run" --repo "$repo" --name "$group-$sha" --dir "$out/$group-$sha"
done
python3 "$root/scripts/ci-artifacts.py" verify "$out" "$sha" "$repo" "$run" "$attempt"
printf 'Source: https://github.com/%s/actions/runs/%s/attempts/%s\n' "$repo" "$run" "$attempt"
