#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "Bitte als root ausführen: sudo sh scripts/install.sh" >&2
    exit 1
fi

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    i386|i486|i586|i686) GOARCH=386 ;;
    *)
        echo "Nicht unterstützte Architektur: $ARCH" >&2
        exit 1
        ;;
esac

BINARY="${1:-$ROOT/dist/heimdrive-linux-$GOARCH}"
if [ ! -f "$BINARY" ]; then
    echo "Binary nicht gefunden: $BINARY" >&2
    echo "Erst bauen mit: sh scripts/build-linux.sh" >&2
    exit 1
fi
if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemctl wurde nicht gefunden. Diese Installation setzt systemd voraus." >&2
    exit 1
fi

SERVICE_USER=heimdrive
DATA_DIR=/var/lib/heimdrive
ENV_FILE=/etc/heimdrive.env

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    NOLOGIN=$(command -v nologin 2>/dev/null || printf '/bin/false')
    useradd --system --home-dir "$DATA_DIR" --shell "$NOLOGIN" "$SERVICE_USER"
fi

install -d -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0750 "$DATA_DIR"
install -m 0755 "$BINARY" /usr/local/bin/heimdrive

NEW_PASSWORD=""
if [ ! -f "$ENV_FILE" ]; then
    if command -v openssl >/dev/null 2>&1; then
        NEW_PASSWORD=$(openssl rand -base64 24 | tr -d '\n=+/' | cut -c1-24)
    else
        NEW_PASSWORD=$(tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24)
    fi
    umask 077
    cat >"$ENV_FILE" <<EOF
HEIMDRIVE_ADDR=:8080
HEIMDRIVE_DATA=$DATA_DIR
HEIMDRIVE_USER=admin
HEIMDRIVE_PASSWORD=$NEW_PASSWORD
HEIMDRIVE_NAME=HeimDrive
EOF
    chmod 600 "$ENV_FILE"
else
    chmod 600 "$ENV_FILE"
fi

install -m 0644 "$ROOT/packaging/heimdrive.service" /etc/systemd/system/heimdrive.service
systemctl daemon-reload
systemctl enable --now heimdrive.service

LAN_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
[ -n "${LAN_IP:-}" ] || LAN_IP="<IP-des-Servers>"

echo
echo "HeimDrive wurde installiert."
echo "  Benutzer: admin"
if [ -n "$NEW_PASSWORD" ]; then
    echo "  Passwort: $NEW_PASSWORD"
else
    echo "  Passwort: steht in $ENV_FILE"
fi
echo "  Adresse:  http://$LAN_IP:8080"
echo
echo "Status:  systemctl status heimdrive"
echo "Logs:    journalctl -u heimdrive -f"
echo "Daten:   $DATA_DIR"
echo
echo "Falls eine Firewall aktiv ist, Port 8080 im lokalen Netzwerk freigeben."
echo "Beispiel mit ufw: sudo ufw allow from 192.168.0.0/16 to any port 8080 proto tcp"
