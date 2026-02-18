#!/bin/sh
set -e

# ekaya-engine installer
# Usage: curl -sSfL https://raw.githubusercontent.com/ekaya-inc/ekaya-engine/main/install.sh | sh
#
# Environment variable overrides:
#   INSTALL_DIR   - where to install the binary (default: /usr/local/bin)
#   VERSION       - specific version to install (default: latest)
#   CONFIG_DIR    - where to create config.yaml (default: ~/.ekaya)

OWNER="ekaya-inc"
REPO="ekaya-engine"
BINARY="ekaya-engine"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-$HOME/.ekaya}"
VERSION="${VERSION:-latest}"

# Colors (only if terminal supports them)
if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  BLUE='\033[0;34m'
  BOLD='\033[1m'
  NC='\033[0m'
else
  RED=''
  GREEN=''
  YELLOW=''
  BLUE=''
  BOLD=''
  NC=''
fi

info() { printf "${BLUE}==>${NC} ${BOLD}%s${NC}\n" "$1"; }
success() { printf "${GREEN}✓${NC} %s\n" "$1"; }
warn() { printf "${YELLOW}⚠${NC}  %s\n" "$1"; }
error() { printf "${RED}✗${NC} %s\n" "$1" >&2; }

# --------------------------------------------------------------------------
# OS and architecture detection
# --------------------------------------------------------------------------

detect_os() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$OS" in
    darwin) OS='darwin' ;;
    linux) OS='linux' ;;
    mingw*|msys*|cygwin*)
      OS='windows'
      warn "Windows detected. Consider using the .zip download from GitHub Releases instead."
      warn "https://github.com/${OWNER}/${REPO}/releases"
      ;;
    *)
      error "Unsupported operating system: $OS"
      error "Supported: macOS (darwin), Linux, Windows"
      exit 1
      ;;
  esac
}

detect_arch() {
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64|amd64) ARCH='amd64' ;;
    aarch64|arm64) ARCH='arm64' ;;
    *)
      error "Unsupported architecture: $ARCH"
      error "Supported: x86_64 (amd64), arm64 (aarch64)"
      exit 1
      ;;
  esac
}

# --------------------------------------------------------------------------
# Download helpers
# --------------------------------------------------------------------------

http_download() {
  _hd_url="$1"
  _hd_dest="$2"

  if command -v curl > /dev/null 2>&1; then
    _hd_code=$(curl -w '%{http_code}' -sL -o "$_hd_dest" "$_hd_url")
    if [ "$_hd_code" != "200" ]; then
      rm -f "$_hd_dest"
      error "Download failed (HTTP $_hd_code): $_hd_url"
      return 1
    fi
  elif command -v wget > /dev/null 2>&1; then
    wget -q -O "$_hd_dest" "$_hd_url" || {
      rm -f "$_hd_dest"
      error "Download failed: $_hd_url"
      return 1
    }
  else
    error "Neither curl nor wget found. Please install one and try again."
    exit 1
  fi
}

http_get() {
  _hg_url="$1"
  if command -v curl > /dev/null 2>&1; then
    curl -sL "$_hg_url"
  elif command -v wget > /dev/null 2>&1; then
    wget -qO- "$_hg_url"
  fi
}

# --------------------------------------------------------------------------
# Checksum verification
# --------------------------------------------------------------------------

verify_checksum() {
  _vc_file="$1"
  _vc_expected="$2"

  if [ -z "$_vc_expected" ]; then
    warn "Could not find checksum for this archive — skipping verification"
    return 0
  fi

  _vc_actual=""
  if command -v sha256sum > /dev/null 2>&1; then
    _vc_actual=$(sha256sum "$_vc_file" | awk '{print $1}')
  elif command -v shasum > /dev/null 2>&1; then
    _vc_actual=$(shasum -a 256 "$_vc_file" | awk '{print $1}')
  elif command -v openssl > /dev/null 2>&1; then
    _vc_actual=$(openssl sha256 "$_vc_file" | awk '{print $NF}')
  else
    warn "No sha256 tool found — skipping checksum verification"
    return 0
  fi

  if [ "$_vc_actual" != "$_vc_expected" ]; then
    error "Checksum verification failed!"
    error "  Expected: $_vc_expected"
    error "  Got:      $_vc_actual"
    exit 1
  fi
  success "Checksum verified"
}

# --------------------------------------------------------------------------
# Interactive configuration setup
# --------------------------------------------------------------------------

generate_secret() {
  if command -v openssl > /dev/null 2>&1; then
    openssl rand -base64 32
  elif command -v dd > /dev/null 2>&1 && [ -e /dev/urandom ]; then
    dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64
  else
    echo "change-me-$(date +%s)-$(od -An -tx1 -N16 /dev/urandom 2>/dev/null | tr -d ' \n' || echo 'fallback')"
  fi
}

setup_config() {
  info "Setting up ekaya-engine configuration"
  echo ""
  echo "Ekaya Engine needs a PostgreSQL database to store its metadata."
  echo "Please provide your PostgreSQL connection details."
  echo ""

  printf "${BOLD}PostgreSQL host${NC} [localhost]: "
  read -r PG_HOST < /dev/tty || true
  PG_HOST="${PG_HOST:-localhost}"

  printf "${BOLD}PostgreSQL port${NC} [5432]: "
  read -r PG_PORT < /dev/tty || true
  PG_PORT="${PG_PORT:-5432}"

  printf "${BOLD}PostgreSQL user${NC} [ekaya]: "
  read -r PG_USER < /dev/tty || true
  PG_USER="${PG_USER:-ekaya}"

  printf "${BOLD}PostgreSQL database${NC} [ekaya_engine]: "
  read -r PG_DATABASE < /dev/tty || true
  PG_DATABASE="${PG_DATABASE:-ekaya_engine}"

  printf "${BOLD}PostgreSQL SSL mode (disable/require/verify-full)${NC} [disable]: "
  read -r PG_SSLMODE < /dev/tty || true
  PG_SSLMODE="${PG_SSLMODE:-disable}"

  echo ""
  printf "${BOLD}PostgreSQL password${NC}: "
  if command -v stty > /dev/null 2>&1; then
    stty -echo 2>/dev/null || true
    read -r PG_PASSWORD < /dev/tty || true
    stty echo 2>/dev/null || true
    printf "\n"
  else
    read -r PG_PASSWORD < /dev/tty || true
  fi

  if [ -z "$PG_PASSWORD" ]; then
    warn "No password set. PostgreSQL may reject connections without a password."
  fi

  echo ""
  info "Generating secrets"
  echo ""
  echo "Ekaya Engine requires two secrets:"
  echo "  1. Credential encryption key — encrypts datasource passwords stored in the database"
  echo "  2. OAuth session secret — signs temporary login cookies"
  echo ""

  DEFAULT_CRED_KEY=$(generate_secret)
  DEFAULT_SESSION_SECRET=$(generate_secret)

  printf "${BOLD}Credential encryption key (auto-generated)${NC} [%s]: " "$DEFAULT_CRED_KEY"
  read -r CRED_KEY < /dev/tty || true
  CRED_KEY="${CRED_KEY:-$DEFAULT_CRED_KEY}"

  printf "${BOLD}OAuth session secret (auto-generated)${NC} [%s]: " "$DEFAULT_SESSION_SECRET"
  read -r SESSION_SECRET < /dev/tty || true
  SESSION_SECRET="${SESSION_SECRET:-$DEFAULT_SESSION_SECRET}"

  echo ""
  printf "${BOLD}Server port${NC} [3443]: "
  read -r SERVER_PORT < /dev/tty || true
  SERVER_PORT="${SERVER_PORT:-3443}"

  # Create config directory
  mkdir -p "$CONFIG_DIR"

  # Write config.yaml
  # Use a quoted heredoc to prevent shell expansion of special characters
  # (e.g. passwords containing $, backticks, or backslashes), then use
  # sed to substitute the collected values.
  GENERATED_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  cat > "$CONFIG_DIR/config.yaml" << 'YAML'
# ekaya-engine configuration
# Generated by install.sh

port: "__SERVER_PORT__"
env: "local"

engine_database:
  pg_host: "__PG_HOST__"
  pg_port: "__PG_PORT__"
  pg_user: "__PG_USER__"
  pg_password: "__PG_PASSWORD__"
  pg_database: "__PG_DATABASE__"
  pg_sslmode: "__PG_SSLMODE__"
  pg_max_connections: 25

project_credentials_key: "__CRED_KEY__"
oauth_session_secret: "__SESSION_SECRET__"

mcp:
  log_requests: true
  log_responses: false
  log_errors: true
YAML

  # Substitute values using a temporary file to handle special characters safely
  _cfg="$CONFIG_DIR/config.yaml"
  _tmp_cfg="${_cfg}.tmp"
  # Use awk for safe substitution (no regex interpretation of replacement values)
  awk -v ph="$PG_HOST" -v pp="$PG_PORT" -v pu="$PG_USER" -v pw="$PG_PASSWORD" \
      -v pd="$PG_DATABASE" -v ps="$PG_SSLMODE" -v sp="$SERVER_PORT" \
      -v ck="$CRED_KEY" -v ss="$SESSION_SECRET" '{
    gsub("__PG_HOST__", ph);
    gsub("__PG_PORT__", pp);
    gsub("__PG_USER__", pu);
    gsub("__PG_PASSWORD__", pw);
    gsub("__PG_DATABASE__", pd);
    gsub("__PG_SSLMODE__", ps);
    gsub("__SERVER_PORT__", sp);
    gsub("__CRED_KEY__", ck);
    gsub("__SESSION_SECRET__", ss);
    print;
  }' "$_cfg" > "$_tmp_cfg" && mv "$_tmp_cfg" "$_cfg"

  success "Configuration written to $CONFIG_DIR/config.yaml"
}

# --------------------------------------------------------------------------
# Main installation
# --------------------------------------------------------------------------

main() {
  echo ""
  printf "${BOLD}Ekaya Engine Installer${NC}\n"
  echo "=============================="
  echo ""

  detect_os
  detect_arch

  # Resolve latest version
  if [ "$VERSION" = "latest" ]; then
    info "Checking latest version..."
    VERSION=$(http_get "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
      | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | head -1)

    if [ -z "$VERSION" ]; then
      error "Could not determine latest version. Set VERSION explicitly:"
      error "  VERSION=v1.0.0 curl -sSfL https://raw.githubusercontent.com/ekaya-inc/ekaya-engine/main/install.sh | sh"
      exit 1
    fi
  fi

  VERSION_NO_V=$(echo "$VERSION" | sed 's/^v//')
  info "Installing ekaya-engine ${VERSION} (${OS}/${ARCH})"

  # Build download URL
  if [ "$OS" = "windows" ]; then
    FILENAME="ekaya-engine_${VERSION_NO_V}_${OS}_${ARCH}.zip"
  else
    FILENAME="ekaya-engine_${VERSION_NO_V}_${OS}_${ARCH}.tar.gz"
  fi

  DOWNLOAD_URL="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/${FILENAME}"
  CHECKSUMS_URL="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/checksums.txt"

  # Download to temp directory
  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT

  info "Downloading ${FILENAME}..."
  http_download "$DOWNLOAD_URL" "${TMPDIR}/${FILENAME}"
  success "Downloaded ${FILENAME}"

  # Download and verify checksum
  info "Verifying checksum..."
  http_download "$CHECKSUMS_URL" "${TMPDIR}/checksums.txt" 2>/dev/null || true
  if [ -f "${TMPDIR}/checksums.txt" ]; then
    EXPECTED=$(grep "${FILENAME}" "${TMPDIR}/checksums.txt" | awk '{print $1}')
    verify_checksum "${TMPDIR}/${FILENAME}" "$EXPECTED"
  else
    warn "Could not download checksums — skipping verification"
  fi

  # Extract
  info "Extracting..."
  if [ "$OS" = "windows" ]; then
    unzip -q "${TMPDIR}/${FILENAME}" -d "$TMPDIR"
  else
    tar xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"
  fi

  # Install binary
  EXTRACT_DIR="${TMPDIR}/ekaya-engine_${VERSION_NO_V}_${OS}_${ARCH}"

  if [ "$OS" = "windows" ]; then
    BINARY_NAME="ekaya-engine.exe"
  else
    BINARY_NAME="ekaya-engine"
  fi

  if [ ! -f "${EXTRACT_DIR}/${BINARY_NAME}" ]; then
    error "Expected binary not found at ${EXTRACT_DIR}/${BINARY_NAME}"
    error "The archive may have a different structure than expected."
    exit 1
  fi

  # Check if we can write to INSTALL_DIR
  if [ ! -d "$INSTALL_DIR" ]; then
    mkdir -p "$INSTALL_DIR" 2>/dev/null || {
      info "Creating $INSTALL_DIR requires elevated permissions"
      sudo mkdir -p "$INSTALL_DIR"
    }
  fi

  if [ -w "$INSTALL_DIR" ]; then
    install -m 755 "${EXTRACT_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    info "Installing to $INSTALL_DIR requires elevated permissions"
    sudo install -m 755 "${EXTRACT_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  fi

  success "Installed ekaya-engine to ${INSTALL_DIR}/${BINARY_NAME}"

  # Verify installation
  if command -v ekaya-engine > /dev/null 2>&1; then
    success "ekaya-engine is on your PATH"
  else
    warn "ekaya-engine is not on your PATH"
    warn "Add this to your shell profile:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  fi

  # Interactive setup
  echo ""
  printf "Would you like to configure ekaya-engine now? [Y/n]: "
  read -r SETUP_ANSWER < /dev/tty || SETUP_ANSWER="y"

  case "$SETUP_ANSWER" in
    [nN]*)
      echo ""
      info "Skipping configuration. To set up later, create ${CONFIG_DIR}/config.yaml"
      echo "  from config.yaml.example and edit with your PostgreSQL details."
      ;;
    *)
      setup_config
      ;;
  esac

  echo ""
  echo "=============================="
  printf "${GREEN}${BOLD}Installation complete!${NC}\n"
  echo "=============================="
  echo ""
  echo "Start ekaya-engine:"
  echo "  ekaya-engine"
  echo ""
  echo "Then open http://localhost:${SERVER_PORT:-3443} in your browser."
  echo ""
  echo "Documentation: https://github.com/${OWNER}/${REPO}"
  echo ""
}

# Wrap in main() to prevent partial download execution
main
