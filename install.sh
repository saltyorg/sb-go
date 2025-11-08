#!/usr/bin/env bash
#
# Saltbox CLI Installer
# This script downloads, verifies, and installs the sb-go binary
#

set -euo pipefail

# Configuration
readonly GITHUB_REPO="saltyorg/sb-go"
readonly BINARY_NAME="sb"
readonly INSTALL_PATH="/usr/local/bin/${BINARY_NAME}"
readonly DOWNLOAD_BINARY_NAME="sb_linux_amd64"
readonly TEMP_DIR=$(mktemp -d)
readonly MIN_BINARY_SIZE=1000000  # 1MB minimum size for sanity check
DOWNLOAD_TOOL=""  # Will be set by check_dependencies
FORCE_DOWNLOAD_TOOL=""  # Can be set by command-line argument

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Cleanup function
cleanup() {
    if [[ -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup EXIT

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Check if running as root or with sudo
check_privileges() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root or with sudo"
        exit 1
    fi
}

# Check if Saltbox already exists
check_saltbox_exists() {
    if [[ -d "/srv/git/saltbox" ]]; then
        log_error "/srv/git/saltbox already exists"
        log_error "This installer is for fresh installations only"
        exit 1
    fi
}

# Detect OS and architecture
detect_platform() {
    local os arch

    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)

    if [[ "${os}" != "linux" ]]; then
        log_error "Unsupported operating system: ${os}"
        log_error "This installer only supports Linux"
        exit 1
    fi

    if [[ "${arch}" != "x86_64" && "${arch}" != "amd64" ]]; then
        log_error "Unsupported architecture: ${arch}"
        log_error "This installer only supports x86_64/amd64"
        exit 1
    fi

    log_info "Detected platform: ${os}/${arch}"
}

# Check for required dependencies
check_dependencies() {
    local missing_deps=()

    # If a specific download tool was forced, check only for that one
    if [[ -n "${FORCE_DOWNLOAD_TOOL}" ]]; then
        if ! command -v "${FORCE_DOWNLOAD_TOOL}" &> /dev/null; then
            log_error "Forced download tool '${FORCE_DOWNLOAD_TOOL}' is not available"
            exit 1
        fi
        DOWNLOAD_TOOL="${FORCE_DOWNLOAD_TOOL}"
        log_info "Using forced download tool: ${DOWNLOAD_TOOL}"
    else
        # Check for either curl or wget
        if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
            missing_deps+=("curl or wget")
        fi

        # Determine which download tool to use
        if command -v curl &> /dev/null; then
            DOWNLOAD_TOOL="curl"
            log_info "Using curl for downloads"
        elif command -v wget &> /dev/null; then
            DOWNLOAD_TOOL="wget"
            log_info "Using wget for downloads"
        fi
    fi

    for cmd in file mktemp; do
        if ! command -v "${cmd}" &> /dev/null; then
            missing_deps+=("${cmd}")
        fi
    done

    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_error "Please install them and try again"
        exit 1
    fi

    log_info "All required dependencies are installed"
}

# Get the latest release version from GitHub
get_latest_version() {
    local version
    local github_api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

    # Try SVM proxy first (cached, no rate limits)
    if [[ "${DOWNLOAD_TOOL}" == "curl" ]]; then
        version=$(curl -sSfL "https://svm.saltbox.dev/version?url=${github_api_url}" 2>/dev/null | grep -o '"tag_name":"[^"]*"' | cut -d'"' -f4 || echo "")
    elif [[ "${DOWNLOAD_TOOL}" == "wget" ]]; then
        version=$(wget -qO- "https://svm.saltbox.dev/version?url=${github_api_url}" 2>/dev/null | grep -o '"tag_name":"[^"]*"' | cut -d'"' -f4 || echo "")
    fi

    # Fallback to GitHub API if SVM fails
    if [[ -z "${version}" ]]; then
        log_warn "SVM proxy unavailable, falling back to GitHub API"
        if [[ "${DOWNLOAD_TOOL}" == "curl" ]]; then
            version=$(curl -sSfL "${github_api_url}" 2>/dev/null | grep -o '"tag_name": "[^"]*"' | cut -d'"' -f4 || echo "")
        elif [[ "${DOWNLOAD_TOOL}" == "wget" ]]; then
            version=$(wget -qO- "${github_api_url}" 2>/dev/null | grep -o '"tag_name": "[^"]*"' | cut -d'"' -f4 || echo "")
        fi

        if [[ -z "${version}" ]]; then
            log_error "Failed to fetch latest release version from both SVM and GitHub API"
            exit 1
        fi
    fi

    echo "${version}"
}

# Download the binary
download_binary() {
    local version="$1"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${DOWNLOAD_BINARY_NAME}"
    local temp_binary="${TEMP_DIR}/${BINARY_NAME}"

    log_info "Downloading ${BINARY_NAME} version ${version}..."
    log_info "Download URL: ${download_url}"

    # Download with retries
    local max_retries=3
    local retry_count=0

    while [[ ${retry_count} -lt ${max_retries} ]]; do
        local download_success=false
        local error_output

        if [[ "${DOWNLOAD_TOOL}" == "curl" ]]; then
            error_output=$(curl -fL --progress-bar --retry 2 --retry-delay 2 \
                -o "${temp_binary}" "${download_url}" 2>&1)
            local exit_code=$?
            if [[ ${exit_code} -eq 0 ]]; then
                download_success=true
            else
                log_error "curl failed with exit code ${exit_code}"
                log_error "curl output: ${error_output}"
            fi
        elif [[ "${DOWNLOAD_TOOL}" == "wget" ]]; then
            error_output=$(wget --show-progress --progress=bar:force:noscroll \
                --tries=2 --waitretry=2 -O "${temp_binary}" "${download_url}" 2>&1)
            local exit_code=$?
            if [[ ${exit_code} -eq 0 ]]; then
                download_success=true
            else
                log_error "wget failed with exit code ${exit_code}"
                log_error "wget output: ${error_output}"
            fi
        fi

        if [[ "${download_success}" == "true" ]]; then
            log_success "Download completed"
            echo "${temp_binary}"
            return 0
        else
            retry_count=$((retry_count + 1))
            if [[ ${retry_count} -lt ${max_retries} ]]; then
                log_warn "Download failed, retrying (${retry_count}/${max_retries})..."
                sleep 2
            fi
        fi
    done

    log_error "Failed to download binary after ${max_retries} attempts"
    log_error "Download URL was: ${download_url}"
    log_error "Target file was: ${temp_binary}"
    exit 1
}

# Verify the downloaded binary
verify_binary() {
    local binary_path="$1"
    local expected_version="$2"

    log_info "Verifying downloaded binary..."

    # Check if file exists
    if [[ ! -f "${binary_path}" ]]; then
        log_error "Binary file not found at ${binary_path}"
        exit 1
    fi

    # Check file size (should be at least 1MB for a Go binary)
    local file_size
    file_size=$(stat -c%s "${binary_path}" 2>/dev/null || stat -f%z "${binary_path}" 2>/dev/null)

    if [[ ${file_size} -lt ${MIN_BINARY_SIZE} ]]; then
        log_error "Binary file size (${file_size} bytes) is too small (expected at least ${MIN_BINARY_SIZE} bytes)"
        log_error "This might indicate a corrupted download or HTML error page"
        exit 1
    fi

    log_info "Binary size: ${file_size} bytes"

    # Check if it's actually a binary file
    local file_type
    file_type=$(file -b "${binary_path}")

    if [[ ! "${file_type}" =~ ELF.*executable ]]; then
        log_error "Downloaded file is not a valid Linux executable"
        log_error "File type: ${file_type}"
        exit 1
    fi

    log_info "File type: ${file_type}"

    # Make it executable
    chmod +x "${binary_path}"

    # Test if binary runs and check version
    log_info "Testing binary execution..."

    local version_output
    if ! version_output=$("${binary_path}" version 2>&1); then
        log_error "Binary failed to execute"
        log_error "Output: ${version_output}"
        exit 1
    fi

    log_info "Version output: ${version_output}"

    # Verify the version output has the expected format
    # The version output format is: "Saltbox CLI version: X.X.X (commit: xxxxx)"
    if [[ ! "${version_output}" =~ "Saltbox CLI version: ".*" (commit: ".+")" ]]; then
        log_error "Version output format is incorrect!"
        log_error "Expected format: 'Saltbox CLI version: X.X.X (commit: xxxxx)'"
        log_error "Got: ${version_output}"
        exit 1
    fi

    # Extract the actual version from output
    local actual_version
    actual_version=$(echo "${version_output}" | grep -oP 'Saltbox CLI version: \K[^ ]+')

    # Verify the version matches what we expected to download
    if [[ "${actual_version}" != "${expected_version}" ]]; then
        log_error "Version mismatch!"
        log_error "Expected version: ${expected_version}"
        log_error "Binary reports version: ${actual_version}"
        exit 1
    fi

    log_success "Binary verification passed (version: ${actual_version})"
}

# Install the binary
install_binary() {
    local binary_path="$1"

    log_info "Installing binary to ${INSTALL_PATH}..."

    # Warn if existing binary found
    if [[ -f "${INSTALL_PATH}" ]]; then
        log_warn "Existing binary found at ${INSTALL_PATH}, overwriting..."
    fi

    # Copy the binary
    if ! cp "${binary_path}" "${INSTALL_PATH}"; then
        log_error "Failed to copy binary to ${INSTALL_PATH}"
        exit 1
    fi

    # Ensure it's executable
    chmod +x "${INSTALL_PATH}"

    # Verify installation
    if [[ ! -x "${INSTALL_PATH}" ]]; then
        log_error "Binary was copied but is not executable at ${INSTALL_PATH}"
        exit 1
    fi

    log_success "Binary installed successfully"
}

# Run the setup command
run_setup() {
    local verbose_flag="$1"

    log_info "Running setup command..."
    echo ""

    if [[ "${verbose_flag}" == "true" ]]; then
        if ! "${INSTALL_PATH}" setup --verbose; then
            log_error "Setup command failed"
            exit 1
        fi
    else
        if ! "${INSTALL_PATH}" setup; then
            log_error "Setup command failed"
            exit 1
        fi
    fi

    echo ""
    log_success "Setup completed successfully"
}

# Parse command line arguments
parse_args() {
    VERBOSE_MODE=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            -v|--verbose)
                VERBOSE_MODE=true
                shift
                ;;
            --force-curl)
                FORCE_DOWNLOAD_TOOL="curl"
                shift
                ;;
            --force-wget)
                FORCE_DOWNLOAD_TOOL="wget"
                shift
                ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  -v, --verbose      Enable verbose output in setup command"
                echo "  --force-curl       Force using curl for downloads"
                echo "  --force-wget       Force using wget for downloads"
                echo "  -h, --help         Show this help message"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                echo "Use -h or --help for usage information"
                exit 1
                ;;
        esac
    done
}

# Main installation flow
main() {
    local version
    local temp_binary

    echo ""
    log_info "Saltbox CLI Installer"
    echo ""

    check_privileges
    check_saltbox_exists
    detect_platform
    check_dependencies

    # Always get the latest version
    log_info "Fetching latest release information..."
    version=$(get_latest_version)
    log_info "Latest version: ${version}"

    # Download binary
    temp_binary=$(download_binary "${version}")
    log_info "Downloaded binary path: '${temp_binary}'"

    # Verify binary
    verify_binary "${temp_binary}" "${version}"

    # Install binary
    install_binary "${temp_binary}"

    # Run setup
    run_setup "${VERBOSE_MODE}"

    echo ""
    log_success "Installation complete!"
    log_info "You can now use the '${BINARY_NAME}' command"
    echo ""
}

# Parse arguments and run main function
parse_args "$@"
main
