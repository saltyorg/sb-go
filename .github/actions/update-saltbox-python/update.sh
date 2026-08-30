#!/usr/bin/env bash

set -euo pipefail

uv_version=$UV_VERSION
if [[ ! "$uv_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Invalid embedded sb-go uv version: $uv_version" >&2
    exit 1
fi
if [[ ! "$PYTHON_MINOR" =~ ^[0-9]+\.[0-9]+$ ]]; then
    echo "Invalid Python minor: $PYTHON_MINOR" >&2
    exit 1
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

archive="$workdir/uv.tar.gz"
# shellcheck disable=SC1091 # GitHub supplies GITHUB_ACTION_PATH at action runtime.
source "$GITHUB_ACTION_PATH/download.sh"
download_with_retry \
    "https://github.com/astral-sh/uv/releases/download/$uv_version/uv-x86_64-unknown-linux-gnu.tar.gz" \
    "$archive"
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
if [[ -z "$candidate" ]]; then
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

saltbox_clone_url=${SALTBOX_CLONE_URL:-"https://github.com/$SALTBOX_REPOSITORY.git"}
git clone --depth=1 "$saltbox_clone_url" "$workdir/saltbox"
cd "$workdir/saltbox"
base_commit=$(git rev-parse HEAD)
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

update_toolchain_files

if git diff --quiet -- .python-version .uv-version .github/renovate.json requirements/requirements-saltbox.txt; then
    echo "Saltbox already contains the requested Python toolchain state"
    exit 0
fi

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add .python-version .uv-version .github/renovate.json requirements/requirements-saltbox.txt
git commit -m "chore(deps): update Python toolchain"

git fetch origin master
remote_commit=$(git rev-parse origin/master)
if [[ "$remote_commit" != "$base_commit" ]]; then
    echo "Saltbox master changed from $base_commit to $remote_commit during toolchain generation; refusing to push stale output" >&2
    exit 1
fi

auth_header=$(printf 'x-access-token:%s' "$SALTBOX_TOKEN" | base64 --wrap=0)
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=http.https://github.com/.extraheader \
GIT_CONFIG_VALUE_0="AUTHORIZATION: basic $auth_header" \
    git push origin HEAD:refs/heads/master
unset auth_header
echo "Updated Saltbox master for sb-go $SB_GO_RELEASE with Python $candidate and uv $uv_version"
