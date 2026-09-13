#!/usr/bin/env bash

# Keep this policy aligned with python/runtime.go. Only managed children are
# isolated: action metadata and Git publication settings stay in the parent.
# Saltbox's .python-version is read explicitly by update.sh, never discovered.
managed_python_workspace() {
    /usr/bin/mktemp -d /tmp/sb-go-python-action.XXXXXXXXXX
}

managed_python_command() (
    set -euo pipefail
    local workspace=$1 kind=$2 command_dir=$3 command=$4
    shift 4
    local variable value part resolved scratch child=""
    local -a parts

    # Resolve operator certificate paths before changing the child's directory.
    for variable in SSL_CERT_FILE SSL_CERT_DIR SSL_CLIENT_CERT REQUESTS_CA_BUNDLE CURL_CA_BUNDLE; do
        value=${!variable:-}
        [[ -n $value ]] || continue
        parts=("$value")
        if [[ $variable == SSL_CERT_DIR ]]; then
            IFS=: read -r -a parts <<< "$value"
            [[ $value != *: ]] || parts+=("")
        fi
        resolved=""
        for part in "${parts[@]}"; do
            if [[ -n $part && $part != /* ]]; then
                part=$(/usr/bin/realpath --canonicalize-missing --no-symlinks -- "$part") || {
                    echo "Cannot resolve relative certificate path in $variable" >&2
                    return 1
                }
            fi
            resolved+="${part}:"
        done
        export "$variable=${resolved%:}"
    done
    while IFS= read -r variable; do
        case "$variable" in
            UV_SYSTEM_CERTS | UV_NATIVE_TLS) ;;
            UV | UV_* | PIP_* | PYTHON* | VIRTUAL_ENV* | CONDA_* | _CONDA_* | __PYVENV_LAUNCHER__)
                unset "$variable"
                ;;
            ANSIBLE_*)
                if [[ $kind == entrypoint && ${command##*/} == ansible* ]]; then
                    unset "$variable"
                fi
                ;;
        esac
    done < <(compgen -e)

    export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
    [[ $workspace == /* && $command_dir == /* && $command == /* ]] || {
        echo "Managed Python workspace, command directory, and executable must be absolute" >&2
        return 1
    }
    mkdir -p -- "$workspace/cache" "$workspace/tmp"
    scratch=$(mktemp -d "$workspace/tmp/command.XXXXXXXXXX")
    trap 'rm -rf -- "$scratch"' EXIT
    # Forward cancellation even when only this shell (not its process group)
    # receives a signal. The EXIT trap owns scratch cleanup in all cases.
    trap 'if [[ -n $child ]]; then kill -TERM "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; fi; exit 143' TERM
    # Bash background children can inherit SIGINT ignored; terminate them on
    # cancellation and retain the caller-facing interrupt exit status.
    trap 'if [[ -n $child ]]; then kill -TERM "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; fi; exit 130' INT
    export UV_CACHE_DIR="$workspace/cache" UV_PYTHON_INSTALL_DIR="$workspace/python"
    export TMPDIR="$scratch" TMP="$scratch" TEMP="$scratch"
    case "$kind" in
        uv) set -- --no-config "$@" ;;
        python) set -- -I "$@" ;;
        entrypoint)
            export PATH="${command%/*}:$PATH" PYTHONNOUSERSITE=1 PYTHONSAFEPATH=1
            if [[ ${command##*/} == ansible* ]]; then
                : > "$scratch/ansible.cfg"
                export ANSIBLE_CONFIG="$scratch/ansible.cfg"
            fi
            ;;
        *) echo "Unknown managed Python command kind: $kind" >&2; return 1 ;;
    esac
    cd -- "$command_dir"
    export PWD="$command_dir"
    "$command" "$@" &
    child=$!
    wait "$child"
)
