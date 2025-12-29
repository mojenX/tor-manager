#!/usr/bin/env bash
set -euo pipefail

# ===============================
#        CONFIG
# ===============================

PROJECT_NAME="mojenx-tor"
INSTALL_DIR="/opt/${PROJECT_NAME}"
DATA_DIR="/var/lib/${PROJECT_NAME}"
LOG_DIR="/var/log/${PROJECT_NAME}"
BIN_PATH="/usr/local/bin/${PROJECT_NAME}"
SERVICE_PATH="/etc/systemd/system"
RUN_USER="mojenX"

GO_VERSION_REQUIRED="1.21"

# ===============================
#        HELPERS
# ===============================

info()    { echo -e "\033[36m[INFO]\033[0m $1"; }
success() { echo -e "\033[32m[SUCCESS]\033[0m $1"; }
warn()    { echo -e "\033[33m[WARN]\033[0m $1"; }
error()   { echo -e "\033[31m[ERROR]\033[0m $1"; exit 1; }

# ===============================
#        ROOT CHECK
# ===============================

if [[ $EUID -ne 0 ]]; then
  error "Run as root"
fi

# ===============================
#        BANNER
# ===============================

cat <<'EOF'
███╗   ███╗  ██████╗      ██╗███████╗███╗   ██╗██╗  ██╗
████╗ ████║ ██╔═══██╗     ██║██╔════╝████╗  ██║╚██╗██╔╝
██╔████╔██║ ██║   ██║     ██║█████╗  ██╔██╗ ██║ ╚███╔╝
██║╚██╔╝██║ ██║   ██║██   ██║██╔══╝  ██║╚██╗██║ ██╔██╗
██║ ╚═╝ ██║ ╚██████╔╝╚█████╔╝███████╗██║ ╚████║██╔╝ ██╗
╚═╝     ╚═╝  ╚═════╝  ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝
        mojenx-tor | Multi Instance Tor Manager
EOF

# ===============================
#        DEPENDENCIES
# ===============================

info "Updating system & installing dependencies..."
apt update
apt install -y \
  tor \
  curl \
  git \
  build-essential \
  ca-certificates

# ===============================
#        GO CHECK
# ===============================

if ! command -v go >/dev/null 2>&1; then
  info "Installing Golang..."
  apt install -y golang
fi

GO_VER=$(go version | awk '{print $3}' | sed 's/go//')
info "Detected Go version: $GO_VER"

# ===============================
#        USER SETUP
# ===============================

if ! id "${RUN_USER}" >/dev/null 2>&1; then
  info "Creating system user ${RUN_USER}"
  useradd -r -s /usr/sbin/nologin "${RUN_USER}"
else
  info "User ${RUN_USER} already exists"
fi

# ===============================
#        DIRECTORIES
# ===============================

info "Creating directories..."
mkdir -p "${INSTALL_DIR}" "${DATA_DIR}" "${LOG_DIR}"

chown -R "${RUN_USER}:${RUN_USER}" "${DATA_DIR}" "${LOG_DIR}"

# ===============================
#        DOWNLOAD SOURCE
# ===============================

info "Cloning project..."
rm -rf "${INSTALL_DIR}"
git clone https://github.com/mojenX/mojenx-tor.git "${INSTALL_DIR}"

cd "${INSTALL_DIR}"

# ===============================
#        BUILD
# ===============================

info "Building ${PROJECT_NAME}..."
go build -o "${BIN_PATH}" main.go
chmod +x "${BIN_PATH}"

success "Binary installed at ${BIN_PATH}"

# ===============================
#        SYSTEMD SERVICE
# ===============================

info "Installing systemd service..."

cat > "${SERVICE_PATH}/${PROJECT_NAME}.service" <<EOF
[Unit]
Description=MojenX Tor Manager
After=network.target tor.service
Wants=network.target

[Service]
User=${RUN_USER}
Group=${RUN_USER}
ExecStart=${BIN_PATH}
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

# ===============================
#        SYSTEMD RELOAD
# ===============================

info "Reloading systemd..."
systemctl daemon-reexec
systemctl daemon-reload
systemctl enable "${PROJECT_NAME}"
systemctl restart "${PROJECT_NAME}"

# ===============================
#        DONE
# ===============================

success "mojenx-tor installed successfully!"
echo
info "Status:"
systemctl --no-pager status "${PROJECT_NAME}" || true

echo
success "SOCKS5 Load Balancer: 127.0.0.1:10000"
warn "Use this port in Xray / Marzban / 3x-ui"
