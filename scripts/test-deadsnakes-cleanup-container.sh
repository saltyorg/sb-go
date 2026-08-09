#!/usr/bin/env bash

set -euo pipefail

readonly script_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly repository_root="$(cd -- "${script_directory}/.." && pwd)"
readonly test_binary="${repository_root}/build/python-cleanup-container.test"
readonly container_image="${SB_GO_DEADSNAKES_TEST_IMAGE:-ubuntu:22.04}"
readonly legacy_packages=(python3.12 python3.12-dev python3.12-venv)

active_container=""

cleanup() {
    if [[ -n "${active_container}" ]]; then
        docker stop "${active_container}" >/dev/null 2>&1 || true
        docker rm "${active_container}" >/dev/null 2>&1 || true
    fi
    if [[ -f "${test_binary}" ]]; then
        unlink "${test_binary}"
    fi
}
trap cleanup EXIT

require_python_version() {
    local executable="$1"
    local expected_prefix="$2"
    local actual_version

    actual_version="$(docker exec "${active_container}" "${executable}" --version)"
    if [[ "${actual_version}" != "${expected_prefix}"* ]]; then
        echo "Expected ${executable} to report ${expected_prefix}.x, got: ${actual_version}" >&2
        return 1
    fi
}

require_package_installed() {
    local package_name="$1"
    local status

    status="$(docker exec "${active_container}" dpkg-query -W '-f=${db:Status-Abbrev}' "${package_name}")"
    if [[ "${status}" != "ii " ]]; then
        echo "Expected ${package_name} to be fully installed, got status: ${status}" >&2
        return 1
    fi
}

require_package_removed() {
    local package_name="$1"
    local status

    status="$(docker exec "${active_container}" dpkg-query -W '-f=${db:Status-Abbrev}' "${package_name}" 2>/dev/null || true)"
    if [[ "${status}" == "ii " ]]; then
        echo "Expected ${package_name} to be removed" >&2
        return 1
    fi
}

run_scenario() {
    local scenario="$1"
    local remove_repository_first="$2"

    active_container="sb-go-deadsnakes-${scenario}-$$"
    echo "Running deadsnakes cleanup scenario: ${scenario}"

    docker run --detach --name "${active_container}" \
        --mount "type=bind,src=${test_binary},dst=/usr/local/bin/python-cleanup.test,readonly" \
        "${container_image}" sleep infinity >/dev/null

    docker exec -e DEBIAN_FRONTEND=noninteractive "${active_container}" apt-get update
    docker exec -e DEBIAN_FRONTEND=noninteractive "${active_container}" \
        apt-get install -y software-properties-common ca-certificates
    docker exec -e DEBIAN_FRONTEND=noninteractive "${active_container}" \
        add-apt-repository -y ppa:deadsnakes/ppa

    # This is the final legacy install set used by saltyorg/sb and pre-uv sb-go.
    docker exec -e DEBIAN_FRONTEND=noninteractive "${active_container}" \
        apt-get install -y "${legacy_packages[@]}"
    docker exec "${active_container}" python3.12 -m ensurepip
    docker exec "${active_container}" install -d /srv/ansible
    docker exec "${active_container}" python3.12 -m venv /srv/ansible/venv

    for package_name in "${legacy_packages[@]}"; do
        require_package_installed "${package_name}"
    done
    require_python_version python3.12 "Python 3.12"
    require_python_version /srv/ansible/venv/bin/python3.12 "Python 3.12"

    if [[ "${remove_repository_first}" == "true" ]]; then
        docker exec -e DEBIAN_FRONTEND=noninteractive "${active_container}" \
            add-apt-repository --remove -y ppa:deadsnakes/ppa
        docker exec -e DEBIAN_FRONTEND=noninteractive "${active_container}" apt-get update

        if docker exec "${active_container}" sh -c \
            "apt-cache policy python3.12 | grep -F 'ppa.launchpadcontent.net/deadsnakes/ppa'"; then
            echo "Deadsnakes remained in apt policy after repository removal" >&2
            return 1
        fi
    fi

    docker exec \
        -e SB_GO_DEADSNAKES_CONTAINER_TEST=1 \
        -e "SB_GO_DEADSNAKES_EXPECT_REMOVED=${legacy_packages[*]}" \
        "${active_container}" /usr/local/bin/python-cleanup.test \
        -test.run '^TestCleanupDeadsnakesContainerIntegration$' -test.v

    for package_name in "${legacy_packages[@]}"; do
        require_package_removed "${package_name}"
    done
    require_python_version python3 "Python 3.10"

    docker stop "${active_container}" >/dev/null
    docker rm "${active_container}" >/dev/null
    active_container=""
}

mkdir -p "${repository_root}/build"
(
    cd "${repository_root}"
    CGO_ENABLED=0 go test -c -o "${test_binary}" ./internal/python
)

run_scenario repository-present false
run_scenario repository-removed true

echo "All deadsnakes cleanup container scenarios passed"
