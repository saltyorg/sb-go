#!/usr/bin/env bash

set -euo pipefail

uv_version=$(tr -d '[:space:]' < "$GITHUB_WORKSPACE/.uv-version")
if [[ ! "$uv_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Invalid sb-go .uv-version: $uv_version" >&2
    exit 1
fi
if [[ ! "$PYTHON_MINOR" =~ ^[0-9]+\.[0-9]+$ ]]; then
    echo "Invalid Python minor: $PYTHON_MINOR" >&2
    exit 1
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

archive="$workdir/uv.tar.gz"
curl --fail --location --silent --show-error \
    "https://github.com/astral-sh/uv/releases/download/$uv_version/uv-x86_64-unknown-linux-gnu.tar.gz" \
    --output "$archive"
tar --extract --gzip --file "$archive" --directory "$workdir"
uv_bin=$(find "$workdir" -type f -path '*/uv' -print -quit)
if [[ -z "$uv_bin" ]]; then
    echo "The uv archive did not contain a uv binary" >&2
    exit 1
fi
chmod 0755 "$uv_bin"
if [[ "$($uv_bin --version)" != "uv $uv_version"* ]]; then
    echo "Downloaded uv does not report version $uv_version" >&2
    exit 1
fi

catalog="$workdir/python-catalog.json"
"$uv_bin" python list "cpython@$PYTHON_MINOR" \
    --all-versions \
    --managed-python \
    --output-format json > "$catalog"

candidate=$(jq -r --arg minor "$PYTHON_MINOR" '
    [
      .[]
      | select(.implementation == "cpython")
      | select(.os == "linux")
      | select(.arch == "x86_64")
      | select(.libc == "gnu")
      | select(.variant == "default")
      | select(.version | startswith($minor + "."))
    ]
    | sort_by(.version_parts.major, .version_parts.minor, .version_parts.patch)
    | last
    | .version // empty
' "$catalog")
candidate_url=$(jq -r --arg version "$candidate" '.[] | select(.version == $version) | .url' "$catalog" | head -n 1)
if [[ -z "$candidate" || -z "$candidate_url" ]]; then
    echo "uv $uv_version did not report a compatible CPython $PYTHON_MINOR Linux x86_64 GNU build" >&2
    exit 1
fi

"$uv_bin" python install \
    --managed-python \
    --no-bin \
    --install-dir "$workdir/python" \
    "$candidate"
python_bin=$(UV_PYTHON_INSTALL_DIR="$workdir/python" "$uv_bin" python find \
    --managed-python \
    --no-project \
    --no-python-downloads \
    "$candidate")
if [[ "$($python_bin --version)" != "Python $candidate" ]]; then
    echo "Installed Python does not report version $candidate" >&2
    exit 1
fi

git clone --depth=1 "https://github.com/$SALTBOX_REPOSITORY.git" "$workdir/saltbox"
cd "$workdir/saltbox"
current=$(tr -d '[:space:]' < .python-version)
if [[ ! "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Saltbox .python-version is invalid: $current" >&2
    exit 1
fi
current_uv=$(tr -d '[:space:]' < .uv-version)
if [[ ! "$current_uv" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Saltbox .uv-version is invalid: $current_uv" >&2
    exit 1
fi
if [[ "$(printf '%s\n%s\n' "$current" "$candidate" | sort -V | tail -n 1)" != "$candidate" ]]; then
    echo "uv $uv_version reports Python $candidate, which would downgrade Saltbox Python $current" >&2
    exit 1
fi
if [[ "$(printf '%s\n%s\n' "$current_uv" "$uv_version" | sort -V | tail -n 1)" != "$uv_version" ]]; then
    echo "sb-go uv $uv_version would downgrade Saltbox's uv requirement $current_uv" >&2
    exit 1
fi
if [[ "$current" == "$candidate" && "$current_uv" == "$uv_version" ]]; then
    echo "Saltbox already uses Python $current with uv $current_uv"
    exit 0
fi

candidate_lock="$workdir/requirements-saltbox.txt"
"$uv_bin" pip compile \
    --python-version "$candidate" \
    --generate-hashes \
    --output-file "$candidate_lock" \
    requirements/requirements-saltbox.in
"$uv_bin" venv \
    --python "$python_bin" \
    --no-project \
    --no-python-downloads \
    "$workdir/preflight-venv"
"$uv_bin" pip sync \
    --python "$workdir/preflight-venv/bin/python" \
    --require-hashes \
    "$candidate_lock"
"$uv_bin" pip check --python "$workdir/preflight-venv/bin/python"
"$workdir/preflight-venv/bin/ansible" --version
"$workdir/preflight-venv/bin/certbot" --version
"$workdir/preflight-venv/bin/apprise" --version

update_toolchain_files() {
    printf '%s\n' "$candidate" > .python-version
    printf '%s\n' "$uv_version" > .uv-version
    python3 - "$uv_version" <<'PY'
import re
import sys
from pathlib import Path

path = Path(".github/renovate.json")
content = path.read_text(encoding="utf-8")
updated, count = re.subn(
    r'("constraints"\s*:\s*\{\s*"uv"\s*:\s*")[^"]+("\s*\})',
    rf"\g<1>{sys.argv[1]}\g<2>",
    content,
    count=1,
)
if count != 1:
    raise SystemExit("Unable to update constraints.uv in Saltbox Renovate configuration")
path.write_text(updated, encoding="utf-8")
PY
    "$uv_bin" pip compile \
        --python-version "$candidate" \
        --generate-hashes \
        --output-file requirements/requirements-saltbox.txt \
        requirements/requirements-saltbox.in
}

if [[ "$DRY_RUN" == "true" ]]; then
    update_toolchain_files
    git diff -- .python-version .uv-version .github/renovate.json requirements/requirements-saltbox.txt
    exit 0
fi

branch="automation/python-$candidate-uv-$uv_version"
if git ls-remote --exit-code --heads origin "$branch" >/dev/null 2>&1; then
    git fetch --depth=1 origin "$branch"
    git checkout -B "$branch" FETCH_HEAD
else
    git checkout -b "$branch"
fi
update_toolchain_files

if git diff --quiet -- .python-version .uv-version .github/renovate.json requirements/requirements-saltbox.txt; then
    echo "The Saltbox automation branch already contains the requested toolchain"
    exit 0
fi

git config user.name "saltyorg automation"
git config user.email "actions@users.noreply.github.com"
git add .python-version .uv-version .github/renovate.json requirements/requirements-saltbox.txt
git commit -m "chore(deps): update Python toolchain"

auth_header=$(printf 'x-access-token:%s' "$GH_TOKEN" | base64 --wrap=0)
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=http.https://github.com/.extraheader \
GIT_CONFIG_VALUE_0="AUTHORIZATION: basic $auth_header" \
    git push origin "HEAD:refs/heads/$branch"
unset auth_header

owner=${SALTBOX_REPOSITORY%%/*}
existing_pr=$(gh api \
    -X GET \
    "repos/$SALTBOX_REPOSITORY/pulls" \
    -f state=open \
    -f head="$owner:$branch" \
    --jq '.[0].html_url // empty')
if [[ -n "$existing_pr" ]]; then
    echo "Updated $existing_pr"
    exit 0
fi

body=$(cat <<EOF
uv $uv_version, released with sb-go $SB_GO_RELEASE, reports CPython $candidate as available for Linux x86_64 GNU.

The automation installed that exact interpreter and successfully synchronized and checked the current Saltbox requirements lock before opening this PR.

Python build: $candidate_url
EOF
)
pr_url=$(gh api \
    -X POST \
    "repos/$SALTBOX_REPOSITORY/pulls" \
    -f title="Update Python toolchain to Python $candidate and uv $uv_version" \
    -f head="$branch" \
    -f base=master \
    -f body="$body" \
    --jq .html_url)
echo "Created $pr_url"
