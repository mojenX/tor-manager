
#!/usr/bin/env bash
set -e

REPO="https://github.com/mojenX/tor-manager"
APP="mojenx-tor"
DIR="/opt/${APP}"
BIN="/usr/local/bin/${APP}"
USER="mojenX"

banner() {
cat <<'EOF'
███╗   ███╗  ██████╗      ██╗███████╗███╗   ██╗██╗  ██╗
████╗ ████║ ██╔═══██╗     ██║██╔════╝████╗  ██║╚██╗██╔╝
██╔████╔██║ ██║   ██║     ██║█████╗  ██╔██╗ ██║ ╚███╔╝
██║╚██╔╝██║ ██║   ██║██   ██║██╔══╝  ██║╚██╗██║ ██╔██╗
██║ ╚═╝ ██║ ╚██████╔╝╚█████╔╝███████╗██║ ╚████║██╔╝ ██╗
╚═╝     ╚═╝  ╚═════╝  ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝
        mojenx-tor | Multi-Instance Tor Manager
EOF
}

banner

[[ $EUID -ne 0 ]] && echo "Run as root" && exit 1

echo "[+] Installing dependencies..."
apt update -y
apt install -y tor git curl golang build-essential ca-certificates

echo "[+] Creating user..."
id "${USER}" &>/dev/null || useradd -r -s /usr/sbin/nologin "${USER}"

echo "[+] Fetching source..."
rm -rf "${DIR}"
git clone "${REPO}" "${DIR}"
cd "${DIR}"

echo "[+] Preparing Go modules..."
[ ! -f go.mod ] && go mod init mojenx-tor
go mod tidy

echo "[+] Building..."
go build -o "${BIN}"
chmod +x "${BIN}"

echo
echo "[✓] Build complete"
echo "[✓] Starting mojenx-tor (Ctrl+C to stop)"
echo "---------------------------------------"

exec "${BIN}"
