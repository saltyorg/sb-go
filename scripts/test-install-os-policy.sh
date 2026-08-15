#!/usr/bin/env bash

set -euo pipefail

readonly SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
readonly REPO_ROOT=$(cd -- "${SCRIPT_DIR}/.." && pwd)

# Source the production installer functions without running a real installation;
# the focused cases below replace all mutating phases.
SB_GO_INSTALL_SOURCE_ONLY=true
source "${REPO_ROOT}/install.sh"
unset SB_GO_INSTALL_SOURCE_ONLY
trap - EXIT

readonly TEST_ROOT=$(mktemp -d)
cleanup_test() {
    rm -rf "${TEST_ROOT}"
}
trap cleanup_test EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_trace_contains() {
    local trace_file="$1"
    local entry="$2"
    grep -Fxq "${entry}" "${trace_file}" || fail "trace is missing '${entry}': $(tr '\n' ' ' < "${trace_file}")"
}

assert_trace_excludes() {
    local trace_file="$1"
    local entry="$2"
    if grep -Fxq "${entry}" "${trace_file}"; then
        fail "trace unexpectedly contains '${entry}': $(tr '\n' ' ' < "${trace_file}")"
    fi
}

run_policy_case() {
    local name="$1"
    local version="$2"
    local repair_mode="$3"
    local expected_status="$4"
    local expected_error="$5"
    local case_dir="${TEST_ROOT}/${name}"
    local os_release_file="${case_dir}/os-release"
    local trace_file="${case_dir}/trace"
    local binary_sentinel="${case_dir}/installed-binary"

    mkdir -p "${case_dir}"
    printf 'ID=ubuntu\nVERSION_ID="%s"\n' "${version}" > "${os_release_file}"
    : > "${trace_file}"
    printf 'original-binary\n' > "${binary_sentinel}"

    set +e
    (
        OS_RELEASE_FILE="${os_release_file}"
        REPAIR_MODE="${repair_mode}"
        VERBOSE_MODE=false
        TEMP_DIR=""

        check_privileges() {
            echo "check_privileges" >> "${trace_file}"
        }
        check_saltbox_exists() {
            echo "check_saltbox_exists" >> "${trace_file}"
        }
        detect_platform() {
            echo "detect_platform" >> "${trace_file}"
        }
        check_dependencies() {
            echo "check_dependencies" >> "${trace_file}"
        }
        initialize_temp_dir() {
            echo "initialize_temp_dir" >> "${trace_file}"
            TEMP_DIR="${case_dir}/installer-temp"
        }
        get_latest_version() {
            echo "get_latest_version" >> "${trace_file}"
            echo "v-test"
        }
        download_binary() {
            echo "download_binary" >> "${trace_file}"
            echo "${case_dir}/fake-download"
        }
        verify_binary() {
            echo "verify_binary" >> "${trace_file}"
        }
        install_binary() {
            echo "install_binary" >> "${trace_file}"
            printf 'installed-binary\n' > "${binary_sentinel}"
        }
        run_setup() {
            echo "run_setup" >> "${trace_file}"
        }

        main
    ) > "${case_dir}/stdout" 2> "${case_dir}/stderr"
    local status=$?
    set -e

    if [[ ${status} -ne ${expected_status} ]]; then
        fail "${name} exit status ${status}, want ${expected_status}; stderr: $(tr '\n' ' ' < "${case_dir}/stderr")"
    fi

    assert_trace_contains "${trace_file}" "check_privileges"
    assert_trace_contains "${trace_file}" "detect_platform"

    if [[ ${expected_status} -eq 0 ]]; then
        for entry in check_dependencies initialize_temp_dir get_latest_version download_binary verify_binary install_binary; do
            assert_trace_contains "${trace_file}" "${entry}"
        done
        grep -Fxq "installed-binary" "${binary_sentinel}" || fail "${name} did not reach the simulated install phase"
        if [[ "${repair_mode}" == "true" ]]; then
            assert_trace_excludes "${trace_file}" "run_setup"
        else
            assert_trace_contains "${trace_file}" "run_setup"
        fi
    else
        for entry in check_dependencies initialize_temp_dir get_latest_version download_binary verify_binary install_binary run_setup; do
            assert_trace_excludes "${trace_file}" "${entry}"
        done
        grep -Fxq "original-binary" "${binary_sentinel}" || fail "${name} mutated the binary sentinel before rejection"
        grep -Fq "${expected_error}" "${case_dir}/stderr" || fail "${name} missing error '${expected_error}'"
    fi
}

run_policy_case "ubuntu-22-fresh" "22.04" false 1 "Unsupported Ubuntu version for fresh installation: 22.04"
run_policy_case "ubuntu-22-repair" "22.04" true 0 ""
run_policy_case "ubuntu-20-fresh" "20.04" false 1 "Unsupported Ubuntu version for fresh installation: 20.04"
run_policy_case "ubuntu-20-repair" "20.04" true 1 "Unsupported Ubuntu version for repair: 20.04"
run_policy_case "ubuntu-24-fresh" "24.04" false 0 ""
run_policy_case "ubuntu-26-fresh" "26.04" false 0 ""

echo "PASS: install.sh Ubuntu policy rejects before mutation and preserves the repair runtime policy"
