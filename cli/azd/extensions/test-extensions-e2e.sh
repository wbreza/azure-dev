#!/bin/bash
# setup-extensions.sh - Automated script for AZD extension development
# This script automates the setup and creation of AZD extensions in all supported languages

set -e

# =====================================================
# CONFIGURABLE VARIABLES - Update these as needed
# =====================================================

# Organization and repo for GitHub releases
GITHUB_OWNER="wbreza"
GITHUB_REPO="azd-extensions" # Typically this would be your fork of azure-dev
GITHUB_FULL_REPO="${GITHUB_OWNER}/${GITHUB_REPO}"

# Base extension metadata - will be used for all extensions with language suffix
EXTENSION_PUBLISHER="contoso"
EXTENSION_NAME="azd.sample"
EXTENSION_VERSION="0.1.0"
EXTENSION_DESCRIPTION="Sample extension for AZD"
EXTENSION_BASE_NAMESPACE="sample"

# Supported languages for extension development
LANGUAGES=("go" "dotnet" "python" "javascript")

# Extension capabilities
CAPABILITIES="custom-commands,lifecycle-events"

# Flag to control whether to publish to registry
PUBLISH_TO_REGISTRY=true

# =====================================================
# HELPER FUNCTIONS
# =====================================================

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Print colored output
print_color() {
    local color="$1"
    local message="$2"

    case "$color" in
    "green") echo -e "\033[0;32m${message}\033[0m" ;;
    "red") echo -e "\033[0;31m${message}\033[0m" ;;
    "yellow") echo -e "\033[0;33m${message}\033[0m" ;;
    "blue") echo -e "\033[0;34m${message}\033[0m" ;;
    *) echo "$message" ;;
    esac
}

# Check prerequisites
check_prerequisites() {
    print_color "blue" "===== Checking Prerequisites ====="

    # Check if azd is installed
    if ! command_exists azd; then
        print_color "red" "Error: 'azd' command not found. Please install Azure Dev CLI."
        print_color "yellow" "Visit: https://learn.microsoft.com/azure/developer/azure-developer-cli/install-azd"
        exit 1
    fi
    print_color "green" "✓ AZD is installed"

    # Check if azd alpha.extensions is enabled
    if [[ "$(azd config get alpha.extensions)" != "on" ]]; then
        print_color "yellow" "Enabling AZD extensions alpha feature..."
        azd config set alpha.extensions on
    fi
    print_color "green" "✓ AZD extensions alpha feature is enabled"

    # Check GitHub CLI
    if ! command_exists gh; then
        print_color "red" "Error: GitHub CLI (gh) is not installed."
        print_color "yellow" "Please install GitHub CLI from: https://cli.github.com/manual/installation"
        exit 1
    fi
    print_color "green" "✓ GitHub CLI is installed"

    # Check GitHub authentication
    if ! gh auth status >/dev/null 2>&1; then
        print_color "yellow" "You need to authenticate with GitHub..."
        gh auth login
    fi
    print_color "green" "✓ GitHub authentication is configured"
}

# Add the development extension source
add_dev_extension_source() {
    print_color "blue" "===== Adding Development Extension Source ====="

    # Check if dev source already exists
    if azd extension source list | grep -q "dev"; then
        print_color "yellow" "Development extension source already exists. Skipping..."
    else
        print_color "yellow" "Adding development extension source..."
        azd extension source add -n dev -t url -l "https://aka.ms/azd/extensions/registry/dev"
        print_color "green" "✓ Development extension source added"
    fi
}

# Install the AZD developer extension
install_dev_extension() {
    print_color "blue" "===== Installing AZD Developer Extension ====="

    # Check if extension is already installed
    if azd extension list --installed | grep -q "microsoft.azd.extensions"; then
        print_color "yellow" "AZD Developer Extension already installed. Skipping..."
    else
        print_color "yellow" "Installing AZD Developer Extension..."
        azd extension install microsoft.azd.extensions
    fi
    print_color "green" "✓ AZD Developer Extension installed"
}

# Create, build, pack, and optionally release/publish extension for a given language
create_extension() {
    local lang="$1"
    local ext_id="${EXTENSION_PUBLISHER}.${EXTENSION_NAME}.${lang}"
    local ext_display_name="${EXTENSION_NAME} (${lang})"
    local ext_namespace="${EXTENSION_BASE_NAMESPACE}-${lang}"
    local release_title="Initial Release v${EXTENSION_VERSION} (${lang})"
    local release_notes="Initial release for ${lang}"

    # Store the original directory to return to it later
    local original_dir="$(pwd)"

    print_color "blue" "===== Creating Extension for ${lang} ====="
    print_color "yellow" "Extension ID: ${ext_id}"
    print_color "yellow" "Extension Display Name: ${ext_display_name}"
    print_color "yellow" "Extension Namespace: ${ext_namespace}"

    # Initialize extension (non-interactive mode with command line parameters)
    print_color "yellow" "Initializing ${lang} extension..."
    azd x init --no-prompt \
        --id "${ext_id}" \
        --name "${ext_display_name}" \
        --namespace "${ext_namespace}" \
        --language "${lang}" \
        --capabilities "${CAPABILITIES}"
    print_color "green" "✓ Extension initialized"

    # Navigate to extension directory
    cd "${ext_id}"

    # Build extension for all platforms
    print_color "yellow" "Building extension for all platforms..."
    azd x build --all
    print_color "green" "✓ Extension built"

    # Package extension
    print_color "yellow" "Packaging extension..."
    azd x pack
    print_color "green" "✓ Extension packaged"

    # Release to GitHub (if specified)
    if [[ -n "${GITHUB_FULL_REPO}" && "${GITHUB_FULL_REPO}" != "your-github-username/azure-dev" ]]; then
        print_color "yellow" "Releasing extension to GitHub..."
        azd x release \
            --repo "${GITHUB_FULL_REPO}" \
            --prerelease \
            --confirm
        print_color "green" "✓ Extension released"

        # Publish to registry (if specified)
        if [[ "${PUBLISH_TO_REGISTRY}" == true ]]; then
            print_color "yellow" "Publishing extension to registry..."
            azd x publish \
                --repo "${GITHUB_FULL_REPO}" \
                print_color "green" "✓ Extension published to registry"
        fi
    else
        print_color "yellow" "Skipping GitHub release and publish (GitHub repo not configured)"
    fi

    # Return to the original directory
    cd "${original_dir}"
}

# =====================================================
# MAIN EXECUTION
# =====================================================

main() {
    print_color "blue" "======================================================"
    print_color "blue" "   AZD Extension Development Test Script"
    print_color "blue" "======================================================"

    # Ensure we start from the right location
    local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    cd "$script_dir"

    # Check prerequisites
    check_prerequisites

    # Add dev extension source
    add_dev_extension_source

    # Install developer extension
    install_dev_extension

    # Create extensions for each language
    for lang in "${LANGUAGES[@]}"; do
        create_extension "$lang"
    done

    print_color "blue" "======================================================"
    print_color "green" "✅ All extensions successfully created!"
    print_color "blue" "======================================================"

    print_color "yellow" "To list your installed extensions:"
    print_color "yellow" "  azd extension list --installed"
}

# Run the main function
main
