
#!/usr/bin/env bash
set -e

#######################################
# CONFIG
#######################################

APP_NAME="mojenx-tor"
RUN_USER="mojenX"
INSTALL_DIR="/opt/${APP_NAME}"
BIN_PATH="/usr/local/bin/${APP_NAME}"
LOG_FILE="/var/log/mojenx-tor.log"
REPO_URL="https://github.com/mojenX/tor-manager"

#######################################
# BASIC CHECKS
#######################################

if [[ $EUID -ne 0 ]]; then
  echo "[ERROR] Please run as root"
  exit 1
fi

#######################################
# BANNER
#######################################

cat <<'EOF'
███╗   ███╗  ██████╗      ██╗███████╗███╗   ██╗██╗  ██╗
████╗ ████║ ██╔═══██╗     ██║██╔════╝████╗  ██║╚██╗██╔╝
██╔████╔██║ ██║   ██║     ██║█████╗  ██╔██╗ ██║ ╚███╔╝
██║╚██╔╝██║ ██║   ██║██   ██║██╔══╝  ██║╚██╗██║ ██╔██╗
██║ ╚═╝ ██║ ╚██████╔╝╚█████╔╝███████╗██║ ╚████║██╔╝ ██╗
╚═╝     ╚═╝  ╚═════╝  ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝
        MOJENX TOR MANAGER INSTALLER
EOF

#######################################
# DEPENDENCIES
#######################################

echo "[+] Updating system & installing dependencies..."
apt update -y
apt install -y \
  tor \
  curl \
  git \
  golang \
  build-essential \
  cron \
  netcat-openbsd \
  ca-certificates

#######################################
# USER
#######################################

echo "[+] Creating user ${RUN_USER}..."
if ! id "${RUN_USER}" &>/dev/null; then
  useradd -r -s /usr/sbin/nologin "${RUN_USER}"
else
  echo "[+] User ${RUN_USER} already exists"
fi

#######################################
# DIRECTORIES
#######################################

echo "[+] Preparing directories..."
mkdir -p "${INSTALL_DIR}"
touch "${LOG_FILE}"
chown root:root "${LOG_FILE}"
chmod 644 "${LOG_FILE}"

#######################################
# FETCH SOURCE
#######################################

echo "[+] Fetching source code..."
rm -rf "${INSTALL_DIR}"
git clone "${REPO_URL}" "${INSTALL_DIR}"
cd "${INSTALL_DIR}"

#######################################
# GO MODULES
#######################################

echo "[+] Preparing Go modules..."
if [[ ! -f go.mod ]]; then
  go mod init "${APP_NAME}"
fi
go mod tidy

#######################################
# BUILD
#######################################

echo "[+] Building ${APP_NAME}..."
go build -o "${BIN_PATH}"
chmod +x "${BIN_PATH}"

#######################################
# TOR CONFIG CHECK
#######################################

echo "[+] Ensuring ControlPort is enabled..."

if ! grep -q "^ControlPort 9051" /etc/tor/torrc; then
  echo "ControlPort 9051" >> /etc/tor/torrc
fi

if ! grep -q "^CookieAuthentication 0" /etc/tor/torrc; then
  echo "CookieAuthentication 0" >> /etc/tor/torrc
fi

systemctl restart tor

#######################################
# FINISH
#######################################

echo
echo "[✓] Installation completed successfully"
echo "[✓] Run Tor Manager with:"
echo
echo "    ${APP_NAME}"
echo
echo "[i] Log file: ${LOG_FILE}"
echo "[i] Tor SOCKS: 127.0.0.1:9050"
echo "[i] Tor CTRL : 127.0.0.1:9051"
echo
echo "----------------------------------------"

exec "${BIN_PATH}"

