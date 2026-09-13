#!/usr/bin/env bash
# Exercise real reconciliation against an actual Saltbox lock, including a
# canonical Python version bump. Never mount the host's managed directories.
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
saltbox_checkout=$(realpath -- "${1:-/srv/git/saltbox}")
python_from=${SB_TEST_PYTHON_FROM:-3.12.13}
python_to=$(tr -d '[:space:]' < "$saltbox_checkout/.python-version")
[[ $python_from =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && $python_to =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && $python_from != "$python_to" ]] || {
    echo "Choose distinct exact starting and canonical target Python versions" >&2
    exit 1
}
workdir=$(mktemp -d /tmp/sb-python-acceptance.XXXXXXXXXX)
container=""
cleanup() {
    local status=$?
    if [[ -n $container ]]; then docker rm -f "$container" >/dev/null; fi
    if [[ $status -eq 0 ]]; then
        rm -rf -- "$workdir"
    else
        echo "Acceptance failed; logs retained at $workdir" >&2
    fi
}
trap cleanup EXIT

(cd "$repo_root" && CGO_ENABLED=0 go build -o "$workdir/reconcile" ./scripts/testdata/python-isolation)
container=$(docker run --detach --label com.saltyorg.sb-go.test=python-isolation \
    ubuntu:24.04@sha256:224a1869083a311ef3f13648a154ba79832fbef6364d31493642ca03082da254 sleep infinity)
docker exec "$container" sh -c 'apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates git libpq-dev gcc' > "$workdir/packages.log" 2>&1
docker cp "$workdir/reconcile" "$container:/usr/local/bin/reconcile" >/dev/null
docker exec "$container" mkdir -p /srv/git/saltbox/requirements /ambient/nested/.venv
docker cp "$saltbox_checkout/requirements/requirements-saltbox.txt" "$container:/srv/git/saltbox/requirements/requirements-saltbox.txt" >/dev/null
docker cp "$saltbox_checkout/.uv-version" "$container:/srv/git/saltbox/.uv-version" >/dev/null
docker exec "$container" sh -c '
    printf "%s\n" "$1" > /srv/git/saltbox/.python-version
    git -C /srv/git/saltbox init -b master
    git -C /srv/git/saltbox add .
    git -C /srv/git/saltbox -c user.name=fixture -c user.email=fixture@example.test commit -m "test: create saltbox acceptance fixture"
    printf "invalid = [" > /ambient/uv.toml
    printf "invalid = [" > /ambient/pyproject.toml
    printf "0.0.0\n" > /ambient/.python-version
    printf "UV_PYTHON=/missing\n" > /ambient/nested/.env
    printf "raise RuntimeError(\"ambient json imported\")\n" > /ambient/nested/json.py
' sh "$python_from" > "$workdir/fixture.log" 2>&1

reconcile() {
    docker exec --workdir /ambient/nested "$container" env \
        UV_CONFIG_FILE=/ambient/missing.toml UV_WORKING_DIR=/ambient/missing-cwd \
        UV_WORKING_DIRECTORY=/ambient/missing-cwd UV_CACHE_DIR=/ambient/missing-cache \
        UV_PYTHON=/ambient/missing-python UV_PYTHON_INSTALL_DIR=/ambient/missing-install \
        UV_INDEX_URL=https://invalid.example PIP_INDEX_URL=https://invalid.example \
        PYTHONHOME=/ambient/missing-python PYTHONPATH=/ambient/nested \
        VIRTUAL_ENV=/ambient/missing-venv CONDA_PREFIX=/ambient/missing-conda \
        HOME=/ambient/missing-home XDG_CACHE_HOME=/ambient/missing-cache \
        TMPDIR=/ambient/missing-tmp TMP=/ambient/missing-tmp TEMP=/ambient/missing-tmp \
        ANSIBLE_CONFIG=/ambient/missing-ansible.cfg \
        /usr/local/bin/reconcile "$@"
}
active() { docker exec "$container" readlink /srv/ansible/venv; }

reconcile > "$workdir/create.log" 2>&1
initial=$(active)
reconcile > "$workdir/reuse.log" 2>&1
[[ $(active) == "$initial" ]] || { echo "Healthy environment was recreated" >&2; exit 1; }
echo "PASS: creation and healthy reuse from hostile environment ($python_from)"

docker exec "$container" sh -c 'printf "%s\n" "$1" > /srv/git/saltbox/.python-version' sh "$python_to"
reconcile > "$workdir/bump.log" 2>&1
[[ $(active) != "$initial" ]]
[[ $(docker exec "$container" /srv/ansible/venv/bin/python3 -I --version) == "Python $python_to" ]]
echo "PASS: canonical .python-version bump selects $python_to"

before_force=$(active)
reconcile force-venv > "$workdir/force-venv.log" 2>&1
[[ $(active) != "$before_force" ]]
before_force=$(active)
python_before=$(docker exec "$container" readlink -f /srv/ansible/venv/bin/python3)
reconcile force-python > "$workdir/force-python.log" 2>&1
[[ $(active) != "$before_force" ]]
[[ $(docker exec "$container" readlink -f /srv/ansible/venv/bin/python3) != "$python_before" ]]
echo "PASS: forced venv and Python recreation"

before_failure=$(active)
docker exec "$container" sh -c 'cp /srv/git/saltbox/requirements/requirements-saltbox.txt /tmp/good-lock; printf "this is invalid requirements\n" > /srv/git/saltbox/requirements/requirements-saltbox.txt'
if reconcile > "$workdir/failure.log" 2>&1; then echo "Invalid lock unexpectedly succeeded" >&2; exit 1; fi
[[ $(active) == "$before_failure" ]]
docker exec "$container" sh -c 'cp /tmp/good-lock /srv/git/saltbox/requirements/requirements-saltbox.txt'
reconcile > "$workdir/recovery.log" 2>&1
[[ $(active) == "$before_failure" ]]
docker exec --workdir / "$container" /usr/local/bin/ansible --version > "$workdir/wrapper.log" 2>&1
[[ $(docker exec "$container" find /tmp -maxdepth 1 -name 'sb-go-python-*' -print) == "" ]]
echo "PASS: failed update preserves active generation; recovery reuses it; scratch cleaned"
