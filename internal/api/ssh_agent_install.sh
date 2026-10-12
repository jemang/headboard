#!/bin/sh
# Installs headboard-agent. Usage: sh install.sh https://headboard.example
# The token comes from HEADBOARD_AGENT_TOKEN, or is read from the terminal.
set -eu

url=${1:-}
case "$url" in
  https://*) ;;
  *) echo "headboard-agent: refusing a non-https Headboard URL: '$url'" >&2; exit 1 ;;
esac
url=${url%/}

[ "$(id -u)" -eq 0 ] || { echo "headboard-agent: run as root" >&2; exit 1; }

case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "headboard-agent: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

# The one-line command from Headboard passes the token in the environment;
# without it, ask on the terminal so it never has to be typed into a command.
token=${HEADBOARD_AGENT_TOKEN:-}
if [ -z "$token" ]; then
  printf 'Agent token (hbk_...): ' > /dev/tty
  # Ctrl-C at the prompt must not leave the terminal without echo.
  trap 'stty echo </dev/tty 2>/dev/null' EXIT INT TERM
  stty -echo < /dev/tty
  read -r token < /dev/tty
  stty echo < /dev/tty
  trap - EXIT INT TERM
  printf '\n' > /dev/tty
fi

# token-check:begin
# Letters, digits, _ and - only: the token goes into a key=value file, where a
# newline would start a line of its own.
case "$token" in
  hbk_|hbk_*[!A-Za-z0-9_-]*) echo "headboard-agent: that is not an agent token" >&2; exit 1 ;;
  hbk_*) ;;
  *) echo "headboard-agent: that is not an agent token" >&2; exit 1 ;;
esac
# token-check:end

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
curl -fsSL --proto '=https' --tlsv1.2 "$url/api/ssh/agent/bin/$arch" -o "$tmp"
install -m 0755 "$tmp" /usr/local/bin/headboard-agent

install -d -m 0700 /etc/headboard-agent
umask 077
cfg=$(mktemp)
trap 'rm -f "$tmp" "$cfg"' EXIT
printf 'url=%s\ntoken=%s\n' "$url" "$token" > "$cfg"
install -m 0600 "$cfg" /etc/headboard-agent/config

cat > /etc/systemd/system/headboard-agent.service <<'EOF'
[Unit]
Description=Headboard SSH key agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/headboard-agent
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable headboard-agent
# restart, not start: on a re-run the old binary and token are still running.
systemctl restart headboard-agent
echo "headboard-agent installed. Follow it with: journalctl -u headboard-agent -f"
echo "Later: headboard-agent update to upgrade, headboard-agent uninstall to remove."
