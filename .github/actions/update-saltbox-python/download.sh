#!/usr/bin/env bash

download_with_retry() {
    local url=$1
    local target=$2
    local max_attempts=4
    local delay=1
    local attempt attempt_file status_file http_status curl_exit

    for ((attempt = 1; attempt <= max_attempts; attempt++)); do
        attempt_file=$(mktemp "${target}.attempt-${attempt}.XXXXXX")
        status_file=$(mktemp "${target}.status-${attempt}.XXXXXX")
        curl_exit=0
        curl --location --silent --show-error \
            --output "$attempt_file" \
            --write-out '%{http_code}' \
            "$url" > "$status_file" || curl_exit=$?
        http_status=$(tr -d '[:space:]' < "$status_file")
        rm -f -- "$status_file"

        if [[ $curl_exit -eq 0 && $http_status == 200 ]]; then
            mv -- "$attempt_file" "$target"
            return 0
        fi
        rm -f -- "$attempt_file"

        if ! download_failure_is_retryable "$curl_exit" "$http_status"; then
            echo "Download failed with non-retryable curl exit $curl_exit and HTTP status $http_status" >&2
            return 1
        fi
        if [[ $attempt -eq $max_attempts ]]; then
            echo "Download failed after $max_attempts attempts (curl exit $curl_exit, HTTP status $http_status)" >&2
            return 1
        fi

        sleep "$delay"
        delay=$((delay * 2))
    done
}

download_failure_is_retryable() {
    local curl_exit=$1
    local http_status=$2

    if [[ $http_status == 403 || $http_status == 429 || $http_status =~ ^5[0-9][0-9]$ ]]; then
        return 0
    fi
    case "$curl_exit" in
        5 | 6 | 7 | 18 | 28 | 35 | 52 | 55 | 56 | 92)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}
