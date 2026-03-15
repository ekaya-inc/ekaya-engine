#!/bin/sh
set -e

# ekaya-engine installer
# Usage: curl --proto '=https' --tlsv1.2 -LsSf https://github.com/ekaya-inc/ekaya-engine/releases/latest/download/install.sh | sh
#
# Environment variable overrides:
#   INSTALL_DIR   - where to install the binary (default: /usr/local/bin)
#   VERSION       - specific version to install (default: latest)
#   CONFIG_DIR    - where to create the system config.yaml (default: ~/.ekaya)

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

expand_path() {
  case "$1" in
    "~")
      printf '%s\n' "$HOME"
      ;;
    "~/"*)
      printf '%s/%s\n' "$HOME" "${1#"~/"}"
      ;;
    *)
      printf '%s\n' "$1"
      ;;
  esac
}

normalize_path_settings() {
  INSTALL_DIR=$(expand_path "$INSTALL_DIR")
  CONFIG_DIR=$(expand_path "$CONFIG_DIR")
  TLS_CERT_PATH=$(expand_path "${TLS_CERT_PATH:-}")
  TLS_KEY_PATH=$(expand_path "${TLS_KEY_PATH:-}")
}

prompt_install_dir() {
  echo ""
  printf "${BOLD}Enter server installation directory${NC} [%s]: " "$INSTALL_DIR"
  read -r INSTALL_DIR_ANSWER < /dev/tty || true
  INSTALL_DIR=$(expand_path "${INSTALL_DIR_ANSWER:-$INSTALL_DIR}")
}

url_scheme() {
  printf '%s' "$1" | sed -nE 's#^([A-Za-z][A-Za-z0-9+.-]*)://.*#\1#p'
}

url_host() {
  printf '%s' "$1" \
    | sed -E 's#^[A-Za-z][A-Za-z0-9+.-]*://##; s#/.*$##' \
    | sed -E 's#:[0-9]+$##'
}

is_local_host() {
  case "$1" in
    localhost|127.0.0.1|::1|\[::1\])
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

prompt_postgres_password() {
  while true; do
    printf "${BOLD}PostgreSQL password${NC}: "
    if command -v stty > /dev/null 2>&1; then
      stty -echo 2>/dev/null || true
      if ! read -r PG_PASSWORD < /dev/tty; then
        stty echo 2>/dev/null || true
        printf "\n"
        error "PostgreSQL password is required."
        exit 1
      fi
      stty echo 2>/dev/null || true
      printf "\n"
    else
      if ! read -r PG_PASSWORD < /dev/tty; then
        error "PostgreSQL password is required."
        exit 1
      fi
    fi

    if [ -n "$PG_PASSWORD" ]; then
      break
    fi

    error "PostgreSQL password is required."
  done
}

write_config_file() {
  _cfg="$1"
  _cfg_dir=$(dirname "$_cfg")

  mkdir -p "$_cfg_dir"

  # Write config.yaml directly using printf to safely handle special
  # characters in passwords (avoids shell expansion of $, `, \, etc.)
  {
    printf '# ekaya-engine configuration\n'
    printf '# Generated by install.sh\n'
    printf '\n'
    printf 'port: %s\n' "$SERVER_PORT"
    printf 'env: "local"\n'
    printf 'base_url: "%s"\n' "$BASE_URL"
    if [ -n "$TLS_LINES" ]; then
      printf '%s\n' "$TLS_LINES"
    fi
    printf '\n'
    printf 'engine_database:\n'
    printf '  pg_host: "%s"\n' "$PG_HOST"
    printf '  pg_port: %s\n' "$PG_PORT"
    printf '  pg_user: "%s"\n' "$PG_USER"
    printf '  pg_password: "%s"\n' "$PG_PASSWORD"
    printf '  pg_database: "%s"\n' "$PG_DATABASE"
    printf '  pg_sslmode: "%s"\n' "$PG_SSLMODE"
    printf '  pg_max_connections: 25\n'
    printf '\n'
    printf 'project_credentials_key: "%s"\n' "$CRED_KEY"
    printf 'oauth_session_secret: "%s"\n' "$SESSION_SECRET"
    printf '\n'
    printf 'mcp:\n'
    printf '  log_requests: true\n'
    printf '  log_responses: false\n'
    printf '  log_errors: true\n'
  } > "$_cfg"

  success "Configuration written to $_cfg"
}

setup_config() {
  info "Setting up ekaya-engine configuration"
  echo ""
  SYSTEM_CONFIG_PATH="${CONFIG_DIR}/config.yaml"
  LOCAL_CONFIG_PATH="$(pwd)/config.yaml"

  if [ -f "$SYSTEM_CONFIG_PATH" ]; then
    echo "Your Ekaya system configuration already exists in: $SYSTEM_CONFIG_PATH"
  else
    echo "Your Ekaya system configuration will be: $SYSTEM_CONFIG_PATH"
  fi
  echo "Local config.yaml files override the system configuration."

  while true; do
    printf "Where do you want the config.yaml file to be installed (system, local, both) [both]: "
    read -r CONFIG_INSTALL_TARGET < /dev/tty || true
    CONFIG_INSTALL_TARGET="${CONFIG_INSTALL_TARGET:-both}"
    CONFIG_INSTALL_TARGET=$(printf '%s' "$CONFIG_INSTALL_TARGET" | tr '[:upper:]' '[:lower:]')

    case "$CONFIG_INSTALL_TARGET" in
      system|local|both)
        break
        ;;
      *)
        error "Please enter system, local, or both."
        ;;
    esac
  done

  echo ""
  echo "Ekaya Engine needs a PostgreSQL database to store its metadata."
  echo "Please provide your PostgreSQL connection details."
  echo ""

  DEFAULT_PG_HOST="${PGHOST:-localhost}"
  DEFAULT_PG_PORT="${PGPORT:-5432}"
  DEFAULT_PG_USER="${PGUSER:-ekaya}"
  DEFAULT_PG_DATABASE="${PGDATABASE:-ekaya_engine}"
  DEFAULT_PG_SSLMODE="${PGSSLMODE:-disable}"

  printf "${BOLD}PostgreSQL host${NC} [%s]: " "$DEFAULT_PG_HOST"
  read -r PG_HOST < /dev/tty || true
  PG_HOST="${PG_HOST:-$DEFAULT_PG_HOST}"

  printf "${BOLD}PostgreSQL port${NC} [%s]: " "$DEFAULT_PG_PORT"
  read -r PG_PORT < /dev/tty || true
  PG_PORT="${PG_PORT:-$DEFAULT_PG_PORT}"

  printf "${BOLD}PostgreSQL user${NC} [%s]: " "$DEFAULT_PG_USER"
  read -r PG_USER < /dev/tty || true
  PG_USER="${PG_USER:-$DEFAULT_PG_USER}"

  printf "${BOLD}PostgreSQL database${NC} [%s]: " "$DEFAULT_PG_DATABASE"
  read -r PG_DATABASE < /dev/tty || true
  PG_DATABASE="${PG_DATABASE:-$DEFAULT_PG_DATABASE}"

  printf "${BOLD}PostgreSQL SSL mode (disable/require/verify-full)${NC} [%s]: " "$DEFAULT_PG_SSLMODE"
  read -r PG_SSLMODE < /dev/tty || true
  PG_SSLMODE="${PG_SSLMODE:-$DEFAULT_PG_SSLMODE}"

  echo ""
  prompt_postgres_password

  echo ""
  info "Generating secrets"
  echo ""
  echo "Ekaya Engine requires two secrets:"
  echo "  1. Credential encryption key — encrypts datasource passwords stored in the database"
  echo "  2. OAuth session secret — signs temporary login cookies"
  echo ""

  if [ -n "${PROJECT_CREDENTIALS_KEY:-}" ]; then
    DEFAULT_CRED_KEY="$PROJECT_CREDENTIALS_KEY"
    CRED_KEY_PROMPT="Credential encryption key"
  else
    DEFAULT_CRED_KEY=$(generate_secret)
    CRED_KEY_PROMPT="Credential encryption key (auto-generated)"
  fi

  if [ -n "${OAUTH_SESSION_SECRET:-}" ]; then
    DEFAULT_SESSION_SECRET="$OAUTH_SESSION_SECRET"
    SESSION_SECRET_PROMPT="OAuth session secret"
  else
    DEFAULT_SESSION_SECRET=$(generate_secret)
    SESSION_SECRET_PROMPT="OAuth session secret (auto-generated)"
  fi

  printf "${BOLD}%s${NC} [%s]: " "$CRED_KEY_PROMPT" "$DEFAULT_CRED_KEY"
  read -r CRED_KEY < /dev/tty || true
  CRED_KEY="${CRED_KEY:-$DEFAULT_CRED_KEY}"

  printf "${BOLD}%s${NC} [%s]: " "$SESSION_SECRET_PROMPT" "$DEFAULT_SESSION_SECRET"
  read -r SESSION_SECRET < /dev/tty || true
  SESSION_SECRET="${SESSION_SECRET:-$DEFAULT_SESSION_SECRET}"

  echo ""
  DEFAULT_SERVER_PORT="${PORT:-3443}"

  printf "${BOLD}Server port${NC} [%s]: " "$DEFAULT_SERVER_PORT"
  read -r SERVER_PORT < /dev/tty || true
  SERVER_PORT="${SERVER_PORT:-$DEFAULT_SERVER_PORT}"

  echo ""
  echo "Ekaya Engine needs a base URL — this is the address users will use"
  echo "to access the server in their browser."
  echo ""
  DEFAULT_BASE_URL="${BASE_URL:-http://localhost:${SERVER_PORT}}"

  while true; do
    printf "${BOLD}Base URL${NC} [%s]: " "$DEFAULT_BASE_URL"
    read -r BASE_URL_INPUT < /dev/tty || true
    BASE_URL="${BASE_URL_INPUT:-$DEFAULT_BASE_URL}"
    BASE_URL_SCHEME=$(url_scheme "$BASE_URL")
    BASE_HOST=$(url_host "$BASE_URL")

    if [ -z "$BASE_URL_SCHEME" ] || [ -z "$BASE_HOST" ]; then
      error "Base URL must include a scheme and host, for example http://localhost:${SERVER_PORT}."
      continue
    fi

    case "$BASE_URL_SCHEME" in
      http|https)
        ;;
      *)
        error "Base URL must start with http:// or https://."
        continue
        ;;
    esac

    if ! is_local_host "$BASE_HOST" && [ "$BASE_URL_SCHEME" != "https" ]; then
      error "Non-localhost deployments must use an https:// base URL."
      continue
    fi

    break
  done

  DEFAULT_TLS_CERT_PATH="${TLS_CERT_PATH:-}"
  DEFAULT_TLS_KEY_PATH="${TLS_KEY_PATH:-}"
  TLS_CERT_PATH=""
  TLS_KEY_PATH=""

  case "$BASE_URL_SCHEME" in
    https)
      echo ""
      echo "HTTPS deployments require TLS certificates."
      echo "OAuth 2.1 with PKCE needs the browser Web Crypto API, which only"
      echo "works in secure contexts (HTTPS or localhost)."
      echo ""

      while true; do
        printf "${BOLD}Path to TLS certificate (PEM)${NC}"
        if [ -n "$DEFAULT_TLS_CERT_PATH" ]; then
          printf " [%s]" "$DEFAULT_TLS_CERT_PATH"
        fi
        printf ": "
        read -r TLS_CERT_PATH_INPUT < /dev/tty || true
        TLS_CERT_PATH=$(expand_path "${TLS_CERT_PATH_INPUT:-$DEFAULT_TLS_CERT_PATH}")
        if [ -z "$TLS_CERT_PATH" ]; then
          error "TLS certificate path is required for HTTPS deployments."
          continue
        fi
        if [ ! -f "$TLS_CERT_PATH" ]; then
          error "File not found: $TLS_CERT_PATH"
          continue
        fi
        # Resolve to absolute path
        TLS_CERT_PATH="$(cd "$(dirname "$TLS_CERT_PATH")" && pwd)/$(basename "$TLS_CERT_PATH")"
        break
      done

      while true; do
        printf "${BOLD}Path to TLS private key (PEM)${NC}"
        if [ -n "$DEFAULT_TLS_KEY_PATH" ]; then
          printf " [%s]" "$DEFAULT_TLS_KEY_PATH"
        fi
        printf ": "
        read -r TLS_KEY_PATH_INPUT < /dev/tty || true
        TLS_KEY_PATH=$(expand_path "${TLS_KEY_PATH_INPUT:-$DEFAULT_TLS_KEY_PATH}")
        if [ -z "$TLS_KEY_PATH" ]; then
          error "TLS key path is required for HTTPS deployments."
          continue
        fi
        if [ ! -f "$TLS_KEY_PATH" ]; then
          error "File not found: $TLS_KEY_PATH"
          continue
        fi
        # Resolve to absolute path
        TLS_KEY_PATH="$(cd "$(dirname "$TLS_KEY_PATH")" && pwd)/$(basename "$TLS_KEY_PATH")"
        break
      done

      success "TLS certificate: $TLS_CERT_PATH"
      success "TLS private key: $TLS_KEY_PATH"
      ;;
  esac

  echo ""
  info "Base URL: $BASE_URL"

  # Build the optional TLS lines
  if [ -n "$TLS_CERT_PATH" ] && [ -n "$TLS_KEY_PATH" ]; then
    TLS_LINES="tls_cert_path: \"${TLS_CERT_PATH}\"
tls_key_path: \"${TLS_KEY_PATH}\""
  else
    TLS_LINES=""
  fi

  case "$CONFIG_INSTALL_TARGET" in
    system)
      write_config_file "$SYSTEM_CONFIG_PATH"
      ;;
    local)
      write_config_file "$LOCAL_CONFIG_PATH"
      ;;
    both)
      write_config_file "$SYSTEM_CONFIG_PATH"
      if [ "$LOCAL_CONFIG_PATH" != "$SYSTEM_CONFIG_PATH" ]; then
        write_config_file "$LOCAL_CONFIG_PATH"
      fi
      ;;
  esac
}

# --------------------------------------------------------------------------
# Main installation
# --------------------------------------------------------------------------

main() {
  echo ""
  printf "${BOLD}Ekaya Engine Installer${NC}\n"
  echo "=============================="
  echo ""

  normalize_path_settings

  detect_os
  detect_arch

  # Resolve latest version
  if [ "$VERSION" = "latest" ]; then
    info "Checking latest version..."
    VERSION=$(http_get "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
      | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | head -1)

    if [ -z "$VERSION" ]; then
      error "Could not determine latest version. Set VERSION explicitly:"
      error "  VERSION=v1.0.0 curl --proto '=https' --tlsv1.2 -LsSf https://github.com/ekaya-inc/ekaya-engine/releases/latest/download/install.sh | sh"
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

  prompt_install_dir

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

  # Verify the installed binary is the one that will be resolved from PATH
  hash -r 2>/dev/null || true
  _found_path=$(command -v ekaya-engine 2>/dev/null || true)
  _install_dir_on_path="false"
  case ":$PATH:" in
    *:"$INSTALL_DIR":*)
      _install_dir_on_path="true"
      ;;
  esac

  case "$_found_path" in
    "${INSTALL_DIR}/"*)
      success "ekaya-engine is on your PATH"
      ;;
    *)
      if [ "$_install_dir_on_path" = "true" ] && [ -n "$_found_path" ]; then
        warn "Another ekaya-engine is earlier on your PATH: $_found_path"
        warn "The newly installed binary is at ${INSTALL_DIR}/${BINARY_NAME}"
      else
        warn "ekaya-engine is not on your PATH"
        warn "Add this to your shell profile:"
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
      fi
      ;;
  esac

  # Interactive setup
  echo ""
  printf "Would you like to configure ekaya-engine now? [Y/n]: "
  read -r SETUP_ANSWER < /dev/tty || SETUP_ANSWER="y"

  case "$SETUP_ANSWER" in
    [nN]*)
      echo ""
      info "Skipping configuration. To set up later, create ./config.yaml or ${CONFIG_DIR}/config.yaml"
      echo "  Local config.yaml files override the system configuration."
      echo "  Start from config.yaml.example and edit with your PostgreSQL details."
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
  echo "Then open ${BASE_URL:-http://localhost:${SERVER_PORT:-3443}} in your browser."
  echo ""
  echo "Documentation: https://github.com/${OWNER}/${REPO}"
  echo ""
}

# Wrap in main() to prevent partial download execution
main
