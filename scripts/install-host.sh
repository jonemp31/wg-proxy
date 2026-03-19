#!/bin/bash
set -euo pipefail

# ============================================================
# WireGuard Proxy Manager — Host Installation Script
# Para: Debian 13 (Trixie)
# ============================================================

echo "=== WireGuard Proxy Manager — Setup do Host ==="
echo ""

if [ "$EUID" -ne 0 ]; then
    echo "ERRO: Execute como root (sudo bash install-host.sh)"
    exit 1
fi

# ----------------------------------------------------------
# 1. Instalar WireGuard
# ----------------------------------------------------------
echo "[1/8] Instalando WireGuard..."
apt-get update -qq
apt-get install -y -qq wireguard wireguard-tools

modprobe wireguard 2>/dev/null || true
if ! lsmod | grep -q wireguard; then
    echo "  AVISO: Módulo WireGuard não carregou. Tentando instalar headers..."
    apt-get install -y -qq linux-headers-$(uname -r)
    modprobe wireguard
fi
echo "  ✓ WireGuard instalado"

# ----------------------------------------------------------
# 2. IP Forwarding
# ----------------------------------------------------------
echo "[2/8] Habilitando IP forwarding..."
sysctl -w net.ipv4.ip_forward=1 >/dev/null
grep -q "^net.ipv4.ip_forward=1" /etc/sysctl.conf 2>/dev/null || \
    echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
echo "  ✓ IP forwarding habilitado"

# ----------------------------------------------------------
# 3. Gerar keypair do servidor
# ----------------------------------------------------------
echo "[3/8] Gerando chaves do servidor WireGuard..."
mkdir -p /etc/wireguard

if [ ! -f /etc/wireguard/server_private.key ]; then
    wg genkey | tee /etc/wireguard/server_private.key | wg pubkey > /etc/wireguard/server_public.key
    chmod 600 /etc/wireguard/server_private.key
    echo "  ✓ Chaves geradas"
else
    echo "  ✓ Chaves já existem"
fi

SERVER_PRIVKEY=$(cat /etc/wireguard/server_private.key)
SERVER_PUBKEY=$(cat /etc/wireguard/server_public.key)

# ----------------------------------------------------------
# 4. Criar config WireGuard
# ----------------------------------------------------------
echo "[4/8] Configurando WireGuard..."
if [ ! -f /etc/wireguard/wg0.conf ]; then
    cat > /etc/wireguard/wg0.conf << WGEOF
[Interface]
Address = 10.0.0.1/22
ListenPort = 51820
PrivateKey = ${SERVER_PRIVKEY}
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -A FORWARD -o wg0 -j ACCEPT
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -D FORWARD -o wg0 -j ACCEPT
WGEOF
    chmod 600 /etc/wireguard/wg0.conf
    echo "  ✓ wg0.conf criado"
else
    echo "  ✓ wg0.conf já existe"
fi

# ----------------------------------------------------------
# 5. Iniciar WireGuard
# ----------------------------------------------------------
echo "[5/8] Iniciando interface WireGuard..."
if ! ip link show wg0 &>/dev/null; then
    wg-quick up wg0
fi
systemctl enable wg-quick@wg0 2>/dev/null
echo "  ✓ Interface wg0 ativa"

# ----------------------------------------------------------
# 6. Firewall
# ----------------------------------------------------------
echo "[6/8] Configurando firewall..."
if command -v ufw &>/dev/null; then
    ufw allow 51820/udp >/dev/null 2>&1
    echo "  ✓ UFW: porta 51820/udp aberta"
else
    echo "  ⚠ UFW não encontrado. Certifique-se que UDP 51820 está aberta."
fi

# ----------------------------------------------------------
# 7. Instalar gost v3
# ----------------------------------------------------------
echo "[7/8] Instalando gost..."
if [ ! -f /usr/local/bin/gost ]; then
    GOST_VERSION="3.0.0-rc10"
    ARCH=$(dpkg --print-architecture)
    case $ARCH in
        amd64) GOST_ARCH="amd64" ;;
        arm64) GOST_ARCH="arm64" ;;
        *)     GOST_ARCH="amd64" ;;
    esac

    GOST_URL="https://github.com/go-gost/gost/releases/download/v${GOST_VERSION}/gost_${GOST_VERSION}_linux_${GOST_ARCH}.tar.gz"
    echo "  Baixando gost v${GOST_VERSION}..."
    if wget -q -O /tmp/gost.tar.gz "${GOST_URL}" 2>/dev/null; then
        tar xzf /tmp/gost.tar.gz -C /tmp/
        install -m 755 /tmp/gost /usr/local/bin/gost
        rm -f /tmp/gost /tmp/gost.tar.gz
        echo "  ✓ gost instalado"
    else
        echo "  ⚠ Download falhou. Instale manualmente:"
        echo "    https://github.com/go-gost/gost/releases"
        echo "    Coloque o binário em /usr/local/bin/gost"
    fi
else
    echo "  ✓ gost já instalado"
fi

# ----------------------------------------------------------
# 8. Preparar diretórios do daemon
# ----------------------------------------------------------
echo "[8/8] Criando diretórios..."
mkdir -p /var/lib/wg-manager
echo "  ✓ Diretórios criados"

# ----------------------------------------------------------
# Resumo
# ----------------------------------------------------------
echo ""
echo "============================================"
echo "  Setup concluído!"
echo "============================================"
echo ""
echo "  Server Public Key: ${SERVER_PUBKEY}"
echo "  Interface: wg0 (10.0.0.1/24)"
echo "  Listen Port: 51820"
echo ""
echo "⚠  IMPORTANTE — IP Dinâmico:"
echo "  Seu IP externo muda diariamente."
echo "  Configure DDNS antes de prosseguir!"
echo ""
echo "  Exemplo com DuckDNS (gratuito):"
echo "  1. Acesse https://www.duckdns.org"
echo "  2. Crie um subdomínio"
echo "  3. Adicione ao crontab:"
echo '     */5 * * * * curl -s "https://www.duckdns.org/update?domains=SEU_DOMINIO&token=SEU_TOKEN&ip=" >/dev/null'
echo ""
echo "  Depois configure WG_ENDPOINT no systemd:"
echo "  /etc/systemd/system/wg-manager.service"
echo "  Environment=WG_ENDPOINT=SEU_DOMINIO.duckdns.org:51820"
echo ""
echo "Próximos passos:"
echo "  1. Configurar DDNS"
echo "  2. Compilar o daemon: cd daemon && make build"
echo "  3. Instalar: make install"
echo "  4. Copiar service: cp deploy/wg-manager.service /etc/systemd/system/"
echo "  5. Editar WG_ENDPOINT no service file"
echo "  6. Iniciar: systemctl enable --now wg-manager"
echo "  7. Gerar ENCRYPTION_KEY: openssl rand -hex 32"
echo "  8. Deploy da stack Docker: cd deploy && docker stack deploy -c docker-compose.yml proxy-manager"
echo ""
