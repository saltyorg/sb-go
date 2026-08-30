#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck disable=SC1091 # The helper is resolved relative to this test at runtime.
source "$script_dir/download.sh"

fail() {
    echo "$*" >&2
    exit 1
}

test_transient_status_retries_with_fresh_files() (
    test_dir=$(mktemp -d)
    trap 'rm -rf "$test_dir"' EXIT
    target="$test_dir/uv.tar.gz"
    curl_attempt=0
    response_codes=(403 503 429 200)
    curl_exits=(0 0 0 0)
    outputs=()
    delays=()

    # shellcheck disable=SC2317 # Called indirectly by download_with_retry.
    curl() {
        curl_attempt=$((curl_attempt + 1))
        local output=""
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --output)
                    output=$2
                    shift 2
                    ;;
                --write-out)
                    shift 2
                    ;;
                *)
                    shift
                    ;;
            esac
        done
        outputs+=("$output")
        if [[ ${response_codes[$((curl_attempt - 1))]} == 200 ]]; then
            printf 'archive' > "$output"
        fi
        printf '%s' "${response_codes[$((curl_attempt - 1))]}"
        return "${curl_exits[$((curl_attempt - 1))]}"
    }

    # shellcheck disable=SC2317 # Called indirectly by download_with_retry.
    sleep() {
        delays+=("$1")
    }

    download_with_retry "https://example.com/uv.tar.gz" "$target"
    [[ $curl_attempt -eq 4 ]] || fail "attempts=$curl_attempt, want 4"
    [[ $(< "$target") == archive ]] || fail "downloaded payload mismatch"
    [[ $(printf '%s\n' "${outputs[@]}" | sort -u | wc -l) -eq 4 ]] || fail "download attempts reused a staging file"
    [[ ${delays[*]} == "1 2 4" ]] || fail "delays=${delays[*]}, want 1 2 4"
    if find "$test_dir" -type f -name '*.attempt-*' -print -quit | grep -q .; then
        fail "failed attempt file was retained"
    fi
)

test_permanent_status_does_not_retry() (
    test_dir=$(mktemp -d)
    trap 'rm -rf "$test_dir"' EXIT
    target="$test_dir/uv.tar.gz"
    curl_attempt=0
    delays=()

    # shellcheck disable=SC2317 # Called indirectly by download_with_retry.
    curl() {
        curl_attempt=$((curl_attempt + 1))
        printf '404'
        return 0
    }

    # shellcheck disable=SC2317 # Called indirectly by download_with_retry.
    sleep() {
        delays+=("$1")
    }

    if download_with_retry "https://example.com/missing.tar.gz" "$target" 2>/dev/null; then
        fail "404 download unexpectedly succeeded"
    fi
    [[ $curl_attempt -eq 1 ]] || fail "attempts=$curl_attempt, want 1"
    [[ ${#delays[@]} -eq 0 ]] || fail "delays=${delays[*]}, want none"
)

test_transient_status_retries_with_fresh_files
test_permanent_status_does_not_retry
