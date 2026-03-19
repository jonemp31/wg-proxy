# Guia Técnico Completo: Sistema de Gerenciamento de Proxies Móveis via WireGuard

## Índice

1. [Visão Geral da Arquitetura](#1-visão-geral-da-arquitetura)
2. [Estrutura do Projeto](#2-estrutura-do-projeto)
3. [Setup da Infraestrutura](#3-setup-da-infraestrutura)
4. [Implementação do WireGuard](#4-implementação-do-wireguard)
5. [Sistema de Proxies](#5-sistema-de-proxies)
6. [Backend — WG Manager Daemon](#6-backend--wg-manager-daemon)
7. [Backend — API de Orquestração](#7-backend--api-de-orquestração)
8. [Dashboard](#8-dashboard)
9. [Integração com a API Go (WhatsMeow)](#9-integração-com-a-api-go-whatsmeow)
10. [Automação Completa](#10-automação-completa)
11. [Monitoramento e Estabilidade](#11-monitoramento-e-estabilidade)
12. [Escalabilidade](#12-escalabilidade)
13. [Boas Práticas de Produção](#13-boas-práticas-de-produção)
14. [Checklist de Execução](#14-checklist-de-execução)

---

## 1. Visão Geral da Arquitetura

### 1.1 O que o sistema faz

Transforma celulares Android (sem root) conectados via 4G/5G em proxies de saída de rede utilizáveis por uma API em Go (WhatsMeow) rodando em uma VPS. Todo o gerenciamento é feito via dashboard web.

### 1.2 Componentes do sistema

| Componente | Onde roda | Tecnologia | Função |
|---|---|---|---|
| WireGuard Server | Host (bare metal) | Kernel module + `wg` CLI | Túnel criptografado UDP entre VPS e celulares |
| Policy Routing | Host (bare metal) | `ip rule` + `iptables` + `ip route` | Direciona tráfego de cada proxy pelo túnel correto |
| SOCKS5 Proxies | Host (bare metal) | `gost` (Go binary) | Expõe cada celular como proxy SOCKS5 com autenticação |
| WG Manager Daemon | Host (bare metal) | Go (binário customizado) | API local que gerencia WireGuard + routing + gost |
| Backend API | Docker Swarm | Go (container) | Orquestração, CRUD de devices, geração de QR, API REST |
| PostgreSQL | Docker Swarm | PostgreSQL 16 | Persistência de estado (devices, métricas, logs) |
| Dashboard | Docker Swarm | React (container nginx) | Interface web para operadores |
| GoWA / WhatsMeow | Docker Swarm | Go (container, :3000) | API WhatsApp que consome os proxies |

### 1.3 Fluxo de dados completo

```
CELULAR (4G/5G)                         VPS (192.168.100.152)
┌──────────────┐                        ┌──────────────────────────────────────┐
│              │   UDP :51820            │                                      │
│  WireGuard   │◄──────────────────────►│  wg0 (10.0.0.1/24)                  │
│  App         │   Túnel criptografado  │       │                              │
│  10.0.0.X    │                        │       ▼                              │
│              │                        │  iptables MARK + ip rule + ip route  │
│  ┌────────┐  │                        │       │                              │
│  │ 4G/5G  │  │                        │       ▼                              │
│  │ NAT    │  │                        │  gost (SOCKS5 :108X)                 │
│  └────────┘  │                        │       │                              │
│              │                        │       ▼                              │
└──────────────┘                        │  WhatsMeow API (:3000)               │
                                        │       │                              │
        ◄───────────────────────────────│  proxy: socks5://127.0.0.1:108X     │
  Tráfego WhatsApp sai pelo IP 4G      │                                      │
                                        └──────────────────────────────────────┘
```

### 1.4 Fluxo detalhado de um pacote

1. WhatsMeow quer conectar a `web.whatsapp.com:443`
2. WhatsMeow envia via SOCKS5 proxy `127.0.0.1:1081`
3. `gost` recebe a conexão na porta 1081
4. Pacote de saída é marcado via iptables (`--set-mark 101`)
5. Kernel consulta `ip rule`: fwmark 101 → lookup table `cel1`
6. Table `cel1`: default via `10.0.0.2` dev `wg0`
7. Pacote é encapsulado pelo WireGuard e enviado via UDP :51820 ao celular
8. Celular recebe, decapsula via WireGuard app
9. Android VPN Service roteia o pacote pela interface 4G/5G
10. Operadora faz NAT e envia para `web.whatsapp.com`
11. Resposta volta: internet → 4G → WireGuard tunnel → VPS → gost → WhatsMeow

### 1.5 Decisões arquiteturais fundamentais

**Por que WireGuard no host e não em Docker:**
- WireGuard precisa de `NET_ADMIN` capability e acesso ao kernel module
- Dentro de Docker exigiria `--net=host` + `--cap-add=NET_ADMIN`, anulando isolamento
- Policy routing (ip rule/route) opera no namespace de rede do host
- Mais estável, menos layers de abstração

**Por que gost no host e não em Docker:**
- Precisa acessar as tabelas de roteamento criadas pelo policy routing
- Precisa que os pacotes sejam marcados corretamente pelo iptables
- Com `--net=host` perderia isolamento de qualquer forma

**Por que um WG Manager Daemon separado:**
- O backend (Docker) não tem acesso direto aos comandos `wg`, `ip`, `iptables` do host
- Montar Docker socket ou usar SSH do container é inseguro e frágil
- Um daemon Go no host expõe uma API controlada e segura via Unix socket
- O backend Docker comunica com o daemon via Unix socket montado como volume

**Por que gost como proxy SOCKS5:**
- Binário Go único, sem dependências
- Suporta SOCKS5 com autenticação user:pass
- Suporta binding a interface específica
- Pode ser iniciado/parado programaticamente
- Leve (~5MB RAM por instância)

---

## 2. Estrutura do Projeto

### 2.1 Layout do repositório

```
wg-proxy-manager/
├── daemon/                          # WG Manager Daemon (roda no HOST)
│   ├── cmd/
│   │   └── wg-manager/
│   │       └── main.go              # Entrypoint do daemon
│   ├── internal/
│   │   ├── api/
│   │   │   ├── server.go            # HTTP server (Unix socket)
│   │   │   └── handlers.go          # Handlers: add/remove peer, start/stop proxy
│   │   ├── wireguard/
│   │   │   ├── manager.go           # Wrapper para comandos `wg`
│   │   │   ├── config.go            # Geração de configs (server + client)
│   │   │   └── keygen.go            # Geração de keypairs
│   │   ├── routing/
│   │   │   ├── policy.go            # Criação de ip rule + ip route tables
│   │   │   └── iptables.go          # Regras iptables (MARK)
│   │   ├── proxy/
│   │   │   ├── gost.go              # Lifecycle do gost (start/stop/status)
│   │   │   └── health.go            # Health check de instâncias gost
│   │   ├── monitor/
│   │   │   ├── peers.go             # Polling de `wg show` + parse
│   │   │   └── metrics.go           # Cálculo de deltas (rx/tx bytes)
│   │   └── config/
│   │       └── config.go            # Configuração do daemon (YAML/ENV)
│   ├── go.mod
│   ├── go.sum
│   └── Makefile
│
├── backend/                         # API de Orquestração (roda em DOCKER)
│   ├── cmd/
│   │   └── api/
│   │       └── main.go              # Entrypoint da API
│   ├── internal/
│   │   ├── api/
│   │   │   ├── server.go            # HTTP server (:8080)
│   │   │   ├── routes.go            # Definição de rotas
│   │   │   └── middleware.go        # Auth, CORS, logging
│   │   ├── handlers/
│   │   │   ├── devices.go           # CRUD de devices
│   │   │   ├── proxies.go           # Listagem/status de proxies
│   │   │   ├── qrcode.go            # Geração de QR Code
│   │   │   └── metrics.go           # Endpoint de métricas
│   │   ├── daemon/
│   │   │   └── client.go            # Client HTTP para o WG Manager Daemon (via Unix socket)
│   │   ├── db/
│   │   │   ├── postgres.go          # Conexão PostgreSQL
│   │   │   ├── migrations.go        # Migrações SQL
│   │   │   └── queries.go           # Queries parametrizadas
│   │   ├── models/
│   │   │   ├── device.go            # Struct Device
│   │   │   ├── proxy.go             # Struct Proxy
│   │   │   └── metrics.go           # Struct Metrics
│   │   ├── websocket/
│   │   │   └── hub.go               # WebSocket hub para real-time no dashboard
│   │   └── config/
│   │       └── config.go            # Configuração (ENV)
│   ├── migrations/
│   │   ├── 001_create_devices.sql
│   │   ├── 002_create_metrics.sql
│   │   └── 003_create_events.sql
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
│
├── dashboard/                       # Frontend React (roda em DOCKER)
│   ├── src/
│   │   ├── App.jsx
│   │   ├── index.jsx
│   │   ├── components/
│   │   │   ├── DeviceList.jsx       # Tabela de devices com status
│   │   │   ├── DeviceCard.jsx       # Card individual com métricas
│   │   │   ├── AddDeviceModal.jsx   # Modal com QR Code
│   │   │   ├── ProxyInfo.jsx        # Info do proxy (porta, user, pass)
│   │   │   ├── TrafficChart.jsx     # Gráfico de tráfego (recharts)
│   │   │   ├── StatusBadge.jsx      # Badge online/offline
│   │   │   └── QRCodeDisplay.jsx    # Renderização do QR Code
│   │   ├── hooks/
│   │   │   ├── useWebSocket.js      # Hook para real-time updates
│   │   │   └── useDevices.js        # Hook para CRUD de devices
│   │   ├── services/
│   │   │   └── api.js               # Client HTTP para o backend
│   │   └── styles/
│   │       └── tailwind.css
│   ├── Dockerfile
│   ├── nginx.conf
│   ├── package.json
│   └── vite.config.js
│
├── deploy/
│   ├── docker-compose.yml           # Stack Docker Swarm (backend + dashboard + postgres)
│   ├── wg-manager.service           # Systemd unit para o daemon no host
│   └── wg0.conf                     # Config base do WireGuard server
│
└── scripts/
    ├── install-host.sh              # Setup do host (WireGuard, gost, daemon)
    ├── install-stack.sh             # Deploy da stack Docker
    └── backup-db.sh                 # Backup do PostgreSQL
```

### 2.2 Comunicação entre componentes

```
┌─────────────────────────────────────────────────────────────────┐
│                           HOST                                  │
│                                                                 │
│   ┌─────────────────────┐     Unix Socket                      │
│   │  WG Manager Daemon  │◄──────────────────┐                  │
│   │  (Go binary)        │     /var/run/      │                  │
│   │                     │     wg-manager.sock│                  │
│   └────────┬────────────┘                    │                  │
│            │                                 │                  │
│            │ executa                         │                  │
│            ▼                                 │                  │
│   ┌────────────────┐  ┌──────────────┐       │                  │
│   │ wg (CLI)       │  │ ip / iptables│       │                  │
│   │ gost (process) │  │ (CLI)        │       │                  │
│   └────────────────┘  └──────────────┘       │                  │
│                                              │                  │
│ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─│─ ─ ─ ─ ─ ─ ─ ─ │
│                    DOCKER SWARM              │                  │
│                                              │                  │
│   ┌──────────────┐  volume mount:            │                  │
│   │  Backend API │  /var/run/wg-manager.sock─┘                  │
│   │  :8080       │                                              │
│   └──────┬───────┘                                              │
│          │                                                      │
│          │ TCP                                                   │
│          ▼                                                      │
│   ┌──────────────┐   ┌──────────────┐   ┌──────────────┐      │
│   │  PostgreSQL  │   │  Dashboard   │   │  GoWA API    │      │
│   │  :5432       │   │  :8000       │   │  :3000       │      │
│   └──────────────┘   └──────────────┘   └──────────────┘      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Ponto crítico:** O Unix socket `/var/run/wg-manager.sock` é o único ponto de contato entre Docker e Host. O backend API faz HTTP requests via esse socket para comandar o daemon.

---

## 3. Setup da Infraestrutura

### 3.1 Pré-requisitos no Host

**Sistema operacional:** Debian 12 (Bookworm) ou Ubuntu 24.04 LTS.

**Pacotes necessários:**
- `wireguard` (kernel module + userspace tools)
- `wireguard-tools` (comando `wg`)
- `iptables` ou `nftables`
- `iproute2` (comandos `ip`)
- `docker` + `docker compose` (já instalado)
- `go` >= 1.22 (para compilar daemon e backend)

**Kernel:** Verificar que o módulo WireGuard está disponível:
```bash
modprobe wireguard
lsmod | grep wireguard
```
Se não estiver, instalar `linux-headers-$(uname -r)` e reinstalar `wireguard-dkms`.

**IP forwarding:** Essencial para que a VPS encaminhe pacotes pelo túnel.
```
net.ipv4.ip_forward = 1
```
Em `/etc/sysctl.conf`, aplicar com `sysctl -p`.

**Firewall:** Porta UDP 51820 deve estar aberta (input) para receber conexões WireGuard dos celulares.

### 3.2 Rede e IPs

| Recurso | Valor |
|---|---|
| IP da VPS (LAN) | 192.168.100.152 |
| IP público da VPS | Obter via `curl ifconfig.me` (necessário para Endpoint no config do celular) |
| Subnet WireGuard | 10.0.0.0/24 |
| IP WireGuard da VPS | 10.0.0.1 |
| IPs dos celulares | 10.0.0.2 a 10.0.0.254 (até 253 devices) |
| Porta WireGuard | UDP 51820 |
| Portas SOCKS5 | 1081 a 1334 (253 possíveis) |
| Porta do daemon | Unix socket (não TCP) |
| Porta do backend | 8080 |
| Porta do dashboard | 8000 |

### 3.3 Docker Swarm Stack

A stack Docker contém apenas os componentes que NÃO precisam de acesso ao host:

**Serviços da stack `proxy-manager`:**

| Service | Imagem | Portas | Volumes | Rede |
|---|---|---|---|---|
| backend | Build local (`./backend/Dockerfile`) | 8080:8080 | `/var/run/wg-manager.sock:/var/run/wg-manager.sock` | proxy-manager-net |
| dashboard | Build local (`./dashboard/Dockerfile`) | 8000:80 | — | proxy-manager-net |
| postgres | postgres:16-alpine | 5432:5432 | `pgdata:/var/lib/postgresql/data` | proxy-manager-net |

**Rede overlay:** `proxy-manager-net` (driver overlay, attachable: true) para comunicação entre os serviços Docker e para que o GoWA (em outra stack) possa se conectar ao backend.

**Volume do Unix socket:** O socket `/var/run/wg-manager.sock` é montado como bind mount no container do backend. Isso permite que o backend faça HTTP requests ao daemon do host sem TCP externo.

### 3.4 Portainer

Criar a stack `proxy-manager` via Portainer com o `docker-compose.yml` do diretório `deploy/`. A stack do GoWA/WhatsMeow continua separada, mas na mesma rede overlay para acessar o backend.

---

## 4. Implementação do WireGuard

### 4.1 Configuração do servidor WireGuard

**Gerar keypair do servidor:**
```bash
wg genkey | tee /etc/wireguard/server_private.key | wg pubkey > /etc/wireguard/server_public.key
chmod 600 /etc/wireguard/server_private.key
```

**Arquivo base `/etc/wireguard/wg0.conf`:**
```ini
[Interface]
Address = 10.0.0.1/24
ListenPort = 51820
PrivateKey = <CONTEÚDO_DE_server_private.key>
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -A FORWARD -o wg0 -j ACCEPT
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -D FORWARD -o wg0 -j ACCEPT
```

**IMPORTANTE:** NÃO usar `wg-quick up wg0` para gerenciamento em produção com peers dinâmicos. Em vez disso:
1. Usar `wg-quick up wg0` apenas UMA VEZ para subir a interface com a config base (sem peers)
2. Habilitar `systemctl enable wg-quick@wg0` para auto-start no boot
3. Todos os peers são adicionados/removidos via `wg set` (hot-reload, sem restart)
4. O `wg-quick` lê o arquivo `.conf` apenas no boot; mudanças dinâmicas via `wg set` não persistem no arquivo

**Persistência de peers:** O daemon é responsável por:
- Adicionar peers via `wg set wg0 peer <pubkey> ...`
- Salvar o estado completo no PostgreSQL
- No boot do daemon, ler o PostgreSQL e re-adicionar todos os peers ativos via `wg set`

Isso evita editar o arquivo `wg0.conf` dinamicamente (que é propenso a race conditions e erros).

### 4.2 Gerenciamento de Peers

**Adicionar peer (executado pelo daemon):**
```bash
wg set wg0 peer <PUBLIC_KEY_DO_CELULAR> \
  preshared-key /tmp/psk_celX \
  allowed-ips 10.0.0.X/32
```

Notas:
- `preshared-key` recebe um ARQUIVO, não o valor direto. O daemon deve escrever o PSK em um arquivo temporário, executar o comando, e apagar o arquivo.
- `allowed-ips 10.0.0.X/32` restringe o peer a um único IP na subnet.
- Sem `endpoint` — o server não sabe o IP do celular antecipadamente. O endpoint é aprendido automaticamente no primeiro handshake.
- Sem `persistent-keepalive` no server — quem envia keepalive é o celular (client).

**Remover peer:**
```bash
wg set wg0 peer <PUBLIC_KEY_DO_CELULAR> remove
```

**Listar peers e status:**
```bash
wg show wg0 dump
```

Formato de saída (tab-separated):
```
<public_key>  <preshared_key>  <endpoint>  <allowed_ips>  <latest_handshake>  <rx_bytes>  <tx_bytes>  <persistent_keepalive>
```

`latest_handshake` é Unix timestamp (0 = nunca conectou).
`endpoint` é `IP:porta` real do celular (atualizado a cada handshake).

### 4.3 Geração de Configs para Celulares

Para cada novo device, o daemon gera:

**Keypair do celular:**
```bash
wg genkey → private_key
echo <private_key> | wg pubkey → public_key
wg genpsk → preshared_key
```

**Config do celular (formato .conf):**
```ini
[Interface]
PrivateKey = <PRIVATE_KEY_DO_CELULAR>
Address = 10.0.0.X/32
DNS = 1.1.1.1

[Peer]
PublicKey = <PUBLIC_KEY_DO_SERVIDOR>
PresharedKey = <PRESHARED_KEY>
Endpoint = <IP_PUBLICO_VPS>:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
```

**Decisão crítica sobre AllowedIPs do celular:**

- `AllowedIPs = 10.0.0.0/24` → O celular só aceita/envia tráfego da subnet WireGuard. O celular continua usando sua internet normal para tudo mais. **Mais restritivo, mais limpo.**
- `AllowedIPs = 0.0.0.0/0` → Todo tráfego do celular passa pelo túnel. **NÃO É O QUE QUEREMOS para o celular em si**, mas é necessário para que o WireGuard app aceite rotear pacotes destinados a IPs externos (como `web.whatsapp.com`) que chegam pela VPS.

**A escolha correta é `AllowedIPs = 0.0.0.0/0` no celular.** Razão: quando a VPS envia um pacote com destino `web.whatsapp.com:443` pelo túnel, o celular precisa aceitar esse pacote. Com `AllowedIPs = 10.0.0.0/24`, o celular rejeitaria o pacote porque o destino não está na subnet. Com `0.0.0.0/0`, o celular aceita qualquer destino.

**Efeito colateral:** Todo o tráfego internet do celular também passa pelo túnel (vai para a VPS e sai pela internet da VPS). Se isso for indesejável, a alternativa é usar `AllowedIPs = 10.0.0.0/24, <IPs_DO_WHATSAPP>/32` — mas os IPs do WhatsApp mudam e são numerosos. Pragmaticamente, `0.0.0.0/0` é a escolha correta para produção.

**ATUALIZAÇÃO IMPORTANTE:** Na verdade, o `AllowedIPs` no CLIENT define quais destinos são roteados pelo túnel E quais pacotes incoming são aceitos. Para o cenário de proxy, o que acontece é:

1. VPS envia pacote `[src=10.0.0.1, dst=web.whatsapp.com]` pelo túnel
2. Celular recebe, decapsula
3. O WireGuard app verifica: o `src` é `10.0.0.1`, que está no range de `AllowedIPs`? Se `AllowedIPs = 0.0.0.0/0`, sim.
4. Pacote aceito → Android VPN service roteia → sai pela 4G

Portanto, `AllowedIPs = 0.0.0.0/0` no config do celular é **obrigatório** para o proxy funcionar.

### 4.4 Integração com QR Code

O app WireGuard para Android aceita scan de QR Code contendo o texto completo da config `.conf`.

**Fluxo no backend:**
1. Receber a string da config do daemon
2. Gerar QR Code da string usando biblioteca Go (`skip2/go-qrcode`)
3. Retornar como imagem PNG base64 para o dashboard
4. Também retornar a string raw para permitir "Copiar config"

**Especificações do QR Code:**
- Nível de correção de erro: `Medium` (equilíbrio entre tamanho e robustez)
- Tamanho: 512x512 pixels é suficiente para scan pelo celular
- O conteúdo é a string literal da config (não URL, não JSON — apenas o texto .conf)

---

## 5. Sistema de Proxies

### 5.1 Estratégia: um gost por celular

Cada celular conectado via WireGuard recebe uma instância dedicada do `gost` na VPS. Essa instância escuta em uma porta SOCKS5 única e roteia todo o tráfego pelo túnel WireGuard correspondente.

**Por que uma instância por celular (e não um proxy único com routing):**
- Isolamento total: se um proxy crashar, não afeta os outros
- Monitoramento individual: cada processo pode ser health-checked independentemente
- Simplicidade de routing: cada instância tem sua marca iptables e tabela de rota
- Fácil de matar/reiniciar individualmente

### 5.2 Instalação do gost

**Download do binário gost v3 (versão em Go):**
```bash
# A versão exata deve ser verificada no GitHub releases
# https://github.com/go-gost/gost/releases
wget https://github.com/go-gost/gost/releases/download/v3.x.x/gost_linux_amd64.tar.gz
tar xzf gost_linux_amd64.tar.gz
mv gost /usr/local/bin/gost
chmod +x /usr/local/bin/gost
```

Verificar: `gost -V`

### 5.3 Execução do gost por celular

**Comando para iniciar proxy do celular X:**
```bash
gost -L "socks5://cel1:SENHA_GERADA@:1081"
```

Onde:
- `cel1` é o username SOCKS5
- `SENHA_GERADA` é a senha gerada automaticamente
- `:1081` é a porta (1080 + device_id)

**PROBLEMA:** Esse comando inicia o gost, mas o tráfego sai pela rota default da VPS, não pelo túnel WireGuard do celular. Para forçar a saída pelo celular, é necessário policy routing.

### 5.4 Policy Routing — O Core Técnico

Para cada celular, o daemon precisa configurar três coisas:

**1. Tabela de roteamento dedicada:**
```bash
# Adicionar entrada no /etc/iproute2/rt_tables (ou usar número direto)
echo "101 cel1" >> /etc/iproute2/rt_tables

# Criar rota default na tabela cel1 via túnel WireGuard
ip route add default dev wg0 via 10.0.0.2 table cel1
```

Onde `10.0.0.2` é o IP WireGuard do celular 1.

**2. Regra de policy routing:**
```bash
ip rule add fwmark 101 table cel1
```

Isso diz: "pacotes marcados com fwmark 101 usam a tabela cel1".

**3. Regra iptables para marcar pacotes do proxy:**
```bash
iptables -t mangle -A OUTPUT -p tcp --sport 1081 -j MARK --set-mark 101
```

Isso diz: "pacotes TCP originados na porta 1081 (onde o gost escuta) recebem marca 101".

**Fluxo resultante:**
1. gost na porta 1081 recebe conexão SOCKS5
2. gost abre conexão TCP para o destino (ex: `web.whatsapp.com:443`)
3. Pacote de saída tem `sport=1081` (porta efêmera do gost — ATENÇÃO, veja nota abaixo)
4. iptables marca o pacote com fwmark 101
5. Kernel consulta ip rule → fwmark 101 → tabela cel1
6. Tabela cel1 → default via 10.0.0.2 → pacote vai pelo túnel WireGuard
7. Celular recebe → encaminha pela 4G

**⚠️ NOTA CRÍTICA SOBRE `--sport`:**

O `--sport 1081` no iptables NÃO funciona como esperado aqui. A porta 1081 é a porta em que o gost ESCUTA conexões SOCKS5 de entrada. Quando o gost faz a conexão de SAÍDA para o destino, ele usa uma porta efêmera aleatória (ex: 45678), não 1081.

**Solução correta — marcar por UID do processo:**

Cada instância gost roda como um usuário Linux dedicado:
```bash
useradd --system --no-create-home --shell /usr/sbin/nologin gost-cel1
```

Iniciar o gost como esse usuário:
```bash
sudo -u gost-cel1 gost -L "socks5://cel1:SENHA@:1081"
```

Regra iptables correta:
```bash
iptables -t mangle -A OUTPUT -m owner --uid-owner gost-cel1 -j MARK --set-mark 101
```

Isso marca TODOS os pacotes de saída do processo gost-cel1, independente da porta.

**Alternativa sem criar usuários — marcar via cgroup:**

Usar `cgroupv2` para isolar cada processo gost e marcar por cgroup. Mais complexo, mas mais limpo se houver muitos dispositivos.

**Alternativa mais simples — usar `gost` com bind de interface:**

Na versão v3 do gost, é possível especificar a interface de saída:
```bash
gost -L "socks5://cel1:SENHA@:1081" -F "forward+socks5://:0?interface=wg0&so.mark=101"
```

Verificar se a flag `interface` e `so.mark` são suportadas na versão do gost. Se sim, isso elimina a necessidade de criar usuários dedicados.

**RECOMENDAÇÃO:** Usar a abordagem de UID do processo (um user por gost) por ser a mais confiável e portável. A abordagem `so.mark` do gost depende de suporte na versão específica.

### 5.5 Gerenciamento de portas e autenticação

**Alocação de portas:**
- Porta base: 1081
- Fórmula: `porta = 1080 + device_id`
- device_id é auto-incremento no PostgreSQL, começando em 1
- Range: 1081 a 1334 (253 devices máximo com subnet /24)

**Geração de credenciais:**
- Username: `cel{device_id}` (ex: `cel1`, `cel2`)
- Password: string aleatória de 32 caracteres (a-zA-Z0-9), gerada via `crypto/rand` em Go
- Armazenadas no PostgreSQL (password em plaintext é aceitável aqui pois é credencial interna, não de usuário final; alternativamente, usar bcrypt se desejar)

**Registry de proxies (mantido pelo backend):**
```json
{
  "device_id": 1,
  "name": "Motorola Cel1",
  "wg_ip": "10.0.0.2",
  "proxy_host": "127.0.0.1",
  "proxy_port": 1081,
  "proxy_user": "cel1",
  "proxy_pass": "aB3x...",
  "status": "online",
  "real_ip": "189.43.12.78",
  "operator": "Vivo",
  "rx_bytes": 1048576,
  "tx_bytes": 524288,
  "last_handshake": "2025-01-15T14:30:00Z",
  "uptime_seconds": 86400
}
```

### 5.6 NAT Masquerade no servidor

**ESSENCIAL:** O tráfego que sai da VPS pelo túnel WireGuard em direção ao celular precisa ter source NAT para que as respostas voltem corretamente.

```bash
iptables -t nat -A POSTROUTING -o wg0 -j MASQUERADE
```

Sem isso, os pacotes chegam ao celular com source IP `127.0.0.1` (do gost) ou o IP interno da VPS, e as respostas não voltam.

---

## 6. Backend — WG Manager Daemon

### 6.1 Responsabilidades

O daemon é um binário Go que roda no host com privilégios root (ou com capabilities `CAP_NET_ADMIN`). Ele expõe uma API HTTP via Unix socket.

**Operações que o daemon executa:**

| Endpoint | Método | Ação |
|---|---|---|
| `/peers` | GET | Lista todos os peers (via `wg show wg0 dump`) |
| `/peers` | POST | Adiciona peer (gera keys, `wg set`, cria routing, inicia gost) |
| `/peers/{id}` | DELETE | Remove peer (remove do wg, limpa routing, mata gost) |
| `/peers/{id}/status` | GET | Status detalhado de um peer |
| `/proxies` | GET | Lista todos os proxies SOCKS5 ativos |
| `/proxies/{id}/restart` | POST | Reinicia instância gost de um proxy |
| `/health` | GET | Health check do daemon |
| `/metrics` | GET | Métricas de todos os peers (handshake, rx, tx) |

### 6.2 API — Contrato detalhado

**POST /peers — Adicionar device:**

Request body:
```json
{
  "name": "Motorola Cel1"
}
```

Ações internas (em ordem):
1. Gerar keypair WireGuard para o celular (`wg genkey`, `wg pubkey`)
2. Gerar preshared key (`wg genpsk`)
3. Calcular próximo IP disponível (query no banco ou manter state interno)
4. Calcular próxima porta SOCKS5 disponível
5. Gerar credenciais SOCKS5 (user/pass)
6. Adicionar peer ao WireGuard: `wg set wg0 peer <pubkey> preshared-key <file> allowed-ips 10.0.0.X/32`
7. Criar usuário Linux: `useradd --system --no-create-home gost-celX`
8. Configurar policy routing:
   - `echo "10X celX" >> /etc/iproute2/rt_tables`
   - `ip route add default dev wg0 via 10.0.0.X table celX`
   - `ip rule add fwmark 10X table celX`
   - `iptables -t mangle -A OUTPUT -m owner --uid-owner gost-celX -j MARK --set-mark 10X`
9. Iniciar gost: `sudo -u gost-celX gost -L "socks5://celX:PASS@:108X"` (como processo filho gerenciado)
10. Gerar config string do celular
11. Retornar tudo

Response:
```json
{
  "device_id": 1,
  "name": "Motorola Cel1",
  "wg_ip": "10.0.0.2",
  "wg_public_key": "abc123...",
  "proxy_port": 1081,
  "proxy_user": "cel1",
  "proxy_pass": "xyz789...",
  "client_config": "[Interface]\nPrivateKey = ...\n...",
  "status": "awaiting_connection"
}
```

**DELETE /peers/{id} — Remover device:**

Ações internas (em ordem):
1. Parar processo gost correspondente (SIGTERM, esperar 5s, SIGKILL)
2. Remover regra iptables: `iptables -t mangle -D OUTPUT -m owner --uid-owner gost-celX -j MARK --set-mark 10X`
3. Remover ip rule: `ip rule del fwmark 10X table celX`
4. Remover ip route: `ip route del default table celX`
5. Remover entrada da rt_tables
6. Remover peer do WireGuard: `wg set wg0 peer <pubkey> remove`
7. Remover usuário Linux: `userdel gost-celX`
8. Retornar confirmação

**GET /metrics — Métricas:**

Executa `wg show wg0 dump`, parseia e retorna:
```json
{
  "peers": [
    {
      "public_key": "abc123...",
      "endpoint": "189.43.12.78:34567",
      "latest_handshake": 1705312200,
      "rx_bytes": 1048576,
      "tx_bytes": 524288,
      "allowed_ips": "10.0.0.2/32"
    }
  ]
}
```

### 6.3 Gerenciamento de processos gost

O daemon precisa gerenciar o lifecycle de múltiplos processos gost. Opções:

**Opção A — Processos filhos diretos:**
O daemon inicia cada gost como `exec.Command` em Go, mantém referência ao `*os.Process`, e monitora via goroutine. Se o gost morrer, o daemon reinicia automaticamente. No shutdown do daemon, envia SIGTERM a todos os filhos.

**Opção B — Systemd units dinâmicos:**
O daemon cria uma unit systemd para cada gost (ex: `gost-cel1.service`). Usa `systemctl start/stop/status`. O systemd gerencia restart automático (`Restart=always`).

**RECOMENDAÇÃO:** Opção A para simplicidade e controle direto. O daemon é o supervisor. Se o daemon reiniciar (após update, por exemplo), ele recria todos os processos gost ao ler o estado do PostgreSQL.

### 6.4 Inicialização do daemon

No boot ou restart do daemon:
1. Ler todos os devices ativos do PostgreSQL (via flag de conexão ou config)
2. Para cada device ativo:
   a. Verificar se o peer existe no WireGuard (`wg show wg0 peers`)
   b. Se não existir, re-adicionar via `wg set`
   c. Verificar se o routing está configurado (`ip rule show`, `ip route show table celX`)
   d. Se não estiver, recriar regras
   e. Verificar se o gost está rodando (tentar bind na porta — se já está ocupada, o processo existe)
   f. Se não estiver, iniciar gost
3. Iniciar goroutine de monitoramento

### 6.5 Unix Socket Server

O daemon escuta em `/var/run/wg-manager.sock` usando `net/http` com `net.Listen("unix", path)`.

**Permissões do socket:** O daemon cria o socket com permissão `0660` e ownership `root:docker` para que o container do backend (que roda como parte do grupo docker) possa acessar.

**No container do backend:** O socket é montado via bind mount no `docker-compose.yml`:
```yaml
volumes:
  - /var/run/wg-manager.sock:/var/run/wg-manager.sock
```

O backend faz requests HTTP usando um `http.Client` com `Transport` configurado para dial via Unix socket:
```go
client := &http.Client{
    Transport: &http.Transport{
        DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
            return net.Dial("unix", "/var/run/wg-manager.sock")
        },
    },
}
resp, err := client.Get("http://localhost/peers")
```

O `http://localhost` é ignorado pelo transport; o dial vai para o Unix socket.

---

## 7. Backend — API de Orquestração

### 7.1 Responsabilidades

A API de orquestração roda em Docker e é o ponto central que o dashboard e outros serviços consomem. Ela NÃO executa comandos no host diretamente — delega ao daemon via Unix socket.

**Responsabilidades:**
- CRUD de devices (com persistência no PostgreSQL)
- Delegação de operações ao daemon
- Geração de QR Code
- Exposição de WebSocket para real-time no dashboard
- Cálculo de métricas derivadas (uptime, velocidade média, etc.)
- Autenticação do dashboard (se necessário)

### 7.2 Endpoints da API REST

| Endpoint | Método | Descrição |
|---|---|---|
| `GET /api/devices` | GET | Lista todos os devices com status atual |
| `POST /api/devices` | POST | Cria novo device (chama daemon, gera QR) |
| `GET /api/devices/{id}` | GET | Detalhes de um device |
| `DELETE /api/devices/{id}` | DELETE | Remove device (chama daemon) |
| `GET /api/devices/{id}/qrcode` | GET | Retorna QR Code PNG do device |
| `GET /api/devices/{id}/config` | GET | Retorna config .conf raw |
| `GET /api/proxies` | GET | Lista proxies disponíveis (online) |
| `GET /api/proxies/random` | GET | Retorna um proxy aleatório online |
| `GET /api/metrics` | GET | Métricas globais |
| `GET /api/metrics/{id}` | GET | Métricas históricas de um device |
| `GET /ws` | WebSocket | Stream de eventos real-time |

### 7.3 Modelo de dados — PostgreSQL

**Tabela `devices`:**
```sql
CREATE TABLE devices (
    id              SERIAL PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    wg_public_key   VARCHAR(44) NOT NULL UNIQUE,     -- Base64 encoded (44 chars)
    wg_private_key  TEXT NOT NULL,                     -- Criptografado (AES-GCM)
    wg_preshared_key TEXT NOT NULL,                    -- Criptografado
    wg_ip           INET NOT NULL UNIQUE,              -- 10.0.0.X
    proxy_port      INTEGER NOT NULL UNIQUE,
    proxy_user      VARCHAR(50) NOT NULL,
    proxy_pass      VARCHAR(100) NOT NULL,             -- Criptografado
    client_config   TEXT NOT NULL,                      -- Config .conf completa
    status          VARCHAR(20) NOT NULL DEFAULT 'awaiting_connection',
    -- status: awaiting_connection, online, offline, disabled
    real_ip         INET,                               -- IP 4G/5G real (do endpoint WireGuard)
    last_handshake  TIMESTAMPTZ,
    rx_bytes        BIGINT NOT NULL DEFAULT 0,
    tx_bytes        BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_devices_status ON devices(status);
CREATE INDEX idx_devices_wg_ip ON devices(wg_ip);
```

**Tabela `device_metrics` (histórico):**
```sql
CREATE TABLE device_metrics (
    id          BIGSERIAL PRIMARY KEY,
    device_id   INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    rx_bytes    BIGINT NOT NULL,          -- Delta desde última medição
    tx_bytes    BIGINT NOT NULL,          -- Delta desde última medição
    latency_ms  INTEGER,                   -- Latência medida via ping pelo túnel
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_device_metrics_device_time ON device_metrics(device_id, recorded_at DESC);

-- Particionamento por tempo (opcional para escala):
-- CREATE TABLE device_metrics (...) PARTITION BY RANGE (recorded_at);
```

**Tabela `device_events` (log de eventos):**
```sql
CREATE TABLE device_events (
    id          BIGSERIAL PRIMARY KEY,
    device_id   INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    event_type  VARCHAR(50) NOT NULL,
    -- event_type: created, connected, disconnected, ip_changed, proxy_restarted, removed
    details     JSONB,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_device_events_device_time ON device_events(device_id, occurred_at DESC);
```

### 7.4 Fluxo do POST /api/devices

1. Receber request com `{ "name": "Motorola Cel1" }`
2. Chamar daemon via Unix socket: `POST http://localhost/peers` com `{ "name": "Motorola Cel1" }`
3. Daemon retorna: keypair, IP, porta, credenciais, config string
4. Persistir no PostgreSQL (tabela `devices`)
5. Gerar QR Code da config string usando `skip2/go-qrcode`
6. Registrar evento `created` na tabela `device_events`
7. Emitir evento WebSocket `device_created` para o dashboard
8. Retornar response com todos os dados + QR Code (base64 PNG)

### 7.5 WebSocket para real-time

O backend mantém um WebSocket hub que broadcast eventos para todos os clients conectados (dashboards).

**Eventos emitidos:**

```json
{"type": "device_created", "device_id": 1, "name": "Motorola Cel1"}
{"type": "device_online", "device_id": 1, "real_ip": "189.43.12.78"}
{"type": "device_offline", "device_id": 1}
{"type": "device_removed", "device_id": 1}
{"type": "metrics_update", "devices": [{"id": 1, "rx_rate": 1024, "tx_rate": 512, "status": "online"}]}
{"type": "ip_changed", "device_id": 1, "old_ip": "189.43.12.78", "new_ip": "189.43.12.99"}
```

**Frequência do metrics_update:** A cada 15 segundos o daemon faz polling do `wg show`, o backend recebe os dados (via polling periódico ao daemon), calcula deltas, e emite via WebSocket.

### 7.6 Criptografia de dados sensíveis

As chaves privadas WireGuard e senhas dos proxies armazenadas no PostgreSQL devem ser criptografadas. Usar AES-256-GCM com uma chave mestra definida como variável de ambiente do backend (`ENCRYPTION_KEY`).

**Em Go:**
```go
// Encrypt: crypto/aes + cipher.NewGCM
// Decrypt: mesma chave + nonce embutido no ciphertext
```

A chave mestra NÃO deve estar no banco, no código, ou no docker-compose. Deve ser fornecida via variável de ambiente ou secrets manager.

---

## 8. Dashboard

### 8.1 Stack do frontend

- **Framework:** React 18+ com Vite
- **Styling:** Tailwind CSS
- **Charts:** Recharts (gráficos de tráfego)
- **QR Code:** `qrcode.react` (renderização client-side para fallback, mas o QR vem do backend como PNG)
- **WebSocket:** Native WebSocket API ou `useWebSocket` hook customizado
- **HTTP Client:** `fetch` nativo (sem necessidade de Axios para essa escala)
- **Build:** Vite → output estático → servido por nginx no container

### 8.2 Telas e componentes

**Tela principal — Lista de devices:**

```
┌──────────────────────────────────────────────────────────────────────┐
│  WG Proxy Manager                              [+ Adicionar Device]  │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │ ● Motorola Cel1          Online     189.43.12.78             │   │
│  │   WG: 10.0.0.2    Proxy: socks5://cel1:abc@127.0.0.1:1081  │   │
│  │   ↑ 1.2 MB/s    ↓ 3.4 MB/s    Uptime: 3d 14h              │   │
│  │   [Copiar Proxy]  [Detalhes]  [Remover]                     │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │ ○ Samsung Cel2           Offline    último: 2h atrás         │   │
│  │   WG: 10.0.0.3    Proxy: socks5://cel2:def@127.0.0.1:1082  │   │
│  │   ↑ 0 B/s       ↓ 0 B/s       Uptime: --                   │   │
│  │   [Copiar Proxy]  [Detalhes]  [Remover]                     │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  Resumo: 2 devices | 1 online | 1 offline                           │
│  Tráfego total: ↑ 14.2 GB  ↓ 42.1 GB                               │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

**Modal — Adicionar device:**

```
┌──────────────────────────────────────────┐
│  Adicionar Novo Dispositivo              │
│                                          │
│  Nome: [Motorola Cel3          ]         │
│                                          │
│  [Gerar Configuração]                    │
│                                          │
│  ┌──────────────────────┐                │
│  │                      │                │
│  │    ██████████████    │                │
│  │    ██          ██    │                │
│  │    ██  QR CODE  ██   │                │
│  │    ██          ██    │                │
│  │    ██████████████    │                │
│  │                      │                │
│  └──────────────────────┘                │
│                                          │
│  Escaneie com o app WireGuard            │
│                                          │
│  [Copiar Config]  [Download .conf]       │
│                                          │
│  Status: ⏳ Aguardando conexão...        │
│  → ● Conectado! Device online.           │
│                                          │
│  Proxy: socks5://cel3:xyz@127.0.0.1:1083│
│  [Copiar URL do proxy]                   │
│                                          │
│  [Fechar]                                │
└──────────────────────────────────────────┘
```

Após gerar, o modal fica aberto e escuta via WebSocket pelo evento `device_online` com o `device_id` correspondente. Quando o celular conecta, o status atualiza automaticamente.

**Tela — Detalhes do device:**

Gráfico de tráfego (upload/download) ao longo do tempo (últimas 24h, 7d, 30d). Tabela de eventos (log de conexões/desconexões/mudanças de IP). Info do proxy com botão de copiar. Botão de reiniciar proxy. Botão de remover device.

### 8.3 Real-time via WebSocket

O dashboard conecta ao WebSocket `ws://192.168.100.152:8080/ws` ao carregar.

**Hook useWebSocket:**
```
- Conecta ao backend
- Recebe eventos JSON
- Atualiza state do React
- Reconecta automaticamente com backoff exponencial (1s, 2s, 4s, 8s, max 30s)
- Emite heartbeat a cada 30s para manter a conexão
```

**Atualização de métricas:**
- O backend emite `metrics_update` a cada 15 segundos
- O dashboard atualiza os valores de tráfego e status de cada card
- Os gráficos de tráfego atualizam com os novos datapoints

### 8.4 Build e deploy

O dashboard é buildado como imagem Docker com multi-stage build:
1. Stage 1: `node:20-alpine` — `npm install && npm run build`
2. Stage 2: `nginx:alpine` — copia os arquivos estáticos do build para `/usr/share/nginx/html`

O `nginx.conf` configura:
- Serve estáticos na porta 80 (mapeada para 8000 no host)
- Proxy pass `/api/*` para o backend (8080) — para que o dashboard acesse a API sem CORS
- Proxy pass `/ws` para o backend WebSocket

---

## 9. Integração com a API Go (WhatsMeow)

### 9.1 Como o WhatsMeow consome proxies

O WhatsMeow usa `gorilla/websocket` para conectar aos servidores do WhatsApp. O websocket dialer aceita um proxy configurável.

**Configuração no código WhatsMeow/GoWA:**
```go
import "golang.org/x/net/proxy"

// Criar dialer SOCKS5
dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1081", 
    &proxy.Auth{User: "cel1", Password: "SENHA"},
    proxy.Direct,
)

// Usar no websocket dialer
wsDialer := websocket.Dialer{
    NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
        return dialer.Dial(network, addr)
    },
}
```

### 9.2 Estratégia de seleção de proxy

**Atribuição fixa (1:1):**
Cada instância WhatsMeow/GoWA é atribuída a um celular fixo. O proxy é configurado no startup da instância. Se o proxy cair, a instância tenta reconectar até o proxy voltar.

**Seleção dinâmica (via API):**
A instância GoWA consulta o backend para obter um proxy disponível:
```
GET http://192.168.100.152:8080/api/proxies/available
```

Retorna lista de proxies online. A instância escolhe um (round-robin, random, ou por afinidade).

**RECOMENDAÇÃO:** Atribuição fixa com fallback dinâmico.
- Cada instância WhatsApp é mapeada a um celular específico no banco de dados
- Se o celular primário ficar offline por mais de X minutos, a instância consulta a API e migra para outro proxy disponível
- Isso minimiza trocas de IP (que podem ser suspeitas para o WhatsApp)

### 9.3 Retry e reconexão

Quando o proxy SOCKS5 retorna erro (connection refused, timeout):
1. WhatsMeow recebe erro no websocket dial
2. Implementar retry com backoff exponencial: 5s, 10s, 20s, 40s, max 60s
3. Após 5 tentativas sem sucesso, consultar a API por proxy alternativo
4. Logar evento de failover para análise

### 9.4 DNS via proxy

**IMPORTANTE:** Usar SOCKS5 com resolução remota (SOCKS5h) para que o DNS resolva no celular (pelo 4G), não na VPS. Isso evita DNS leaks e garante que o WhatsApp veja resolução DNS consistente com o IP de saída.

No `gorilla/websocket`, o SOCKS5 dial já faz resolução remota por padrão quando usando `proxy.SOCKS5()`. Verificar que o endereço passado é hostname (não IP): `web.whatsapp.com:443`, não `157.240.1.1:443`.

---

## 10. Automação Completa

### 10.1 Fluxo "Adicionar dispositivo" — Passo a passo completo

```
OPERADOR                  DASHBOARD               BACKEND              DAEMON                HOST
   │                          │                      │                    │                    │
   │  Clica "Adicionar"       │                      │                    │                    │
   │─────────────────────────►│                      │                    │                    │
   │                          │  POST /api/devices   │                    │                    │
   │                          │─────────────────────►│                    │                    │
   │                          │                      │  POST /peers       │                    │
   │                          │                      │─────────────────►│                    │
   │                          │                      │                    │  wg genkey         │
   │                          │                      │                    │─────────────────►│
   │                          │                      │                    │  wg set wg0 peer   │
   │                          │                      │                    │─────────────────►│
   │                          │                      │                    │  ip route/rule     │
   │                          │                      │                    │─────────────────►│
   │                          │                      │                    │  start gost        │
   │                          │                      │                    │─────────────────►│
   │                          │                      │  {config, keys}    │                    │
   │                          │                      │◄─────────────────│                    │
   │                          │                      │  INSERT devices    │                    │
   │                          │                      │  gera QR code      │                    │
   │                          │  {device, qr_base64} │                    │                    │
   │                          │◄─────────────────────│                    │                    │
   │  Exibe QR Code           │                      │                    │                    │
   │◄─────────────────────────│                      │                    │                    │
   │                          │                      │                    │                    │
   │  Escaneia QR no celular  │                      │                    │                    │
   │  (WireGuard app)         │                      │                    │                    │
   │                          │                      │                    │                    │
   │                          │                      │   polling wg show  │                    │
   │                          │                      │   detecta handshake│                    │
   │                          │  WS: device_online   │                    │                    │
   │                          │◄─────────────────────│                    │                    │
   │  Status: "Online" ✓      │                      │                    │                    │
   │◄─────────────────────────│                      │                    │                    │
```

### 10.2 Fluxo "Remover dispositivo"

1. Operador clica "Remover" no dashboard
2. Dashboard exibe confirmação (modal: "Tem certeza?")
3. Operador confirma
4. Dashboard chama `DELETE /api/devices/{id}`
5. Backend chama `DELETE /peers/{id}` no daemon
6. Daemon executa cleanup completo (gost → iptables → ip rule → ip route → wg peer → user Linux)
7. Backend marca device como `removed` no PostgreSQL (soft delete) ou deleta
8. Backend emite WebSocket `device_removed`
9. Dashboard remove o card do device

### 10.3 Fluxo de reconexão automática após queda

```
CELULAR                           DAEMON (monitoramento)
   │                                  │
   │  Perde sinal 4G                  │
   │  (handshake para de atualizar)   │
   │                                  │
   │                                  │  polling wg show (a cada 15s)
   │                                  │  detecta: last_handshake > 180s
   │                                  │  → marca device como "offline"
   │                                  │  → emite evento via API
   │                                  │
   │  Sinal volta                     │
   │  WireGuard reconnecta (auto)     │
   │  ──────────────────────────────►│
   │  (primeiro keepalive / pacote)   │
   │                                  │
   │                                  │  polling wg show
   │                                  │  detecta: novo handshake
   │                                  │  → marca device como "online"
   │                                  │  → verifica se IP mudou
   │                                  │  → emite evento via API
```

O proxy gost NÃO precisa reiniciar. Ele continua escutando na porta. Quando o túnel volta a funcionar, os pacotes fluem novamente automaticamente.

---

## 11. Monitoramento e Estabilidade

### 11.1 Monitoramento de peers (goroutine no daemon)

**A cada 15 segundos:**
1. Executar `wg show wg0 dump`
2. Parsear output (tab-separated)
3. Para cada peer:
   - Calcular delta de rx/tx bytes desde última medição
   - Verificar `latest_handshake`:
     - Se `now - latest_handshake < 180s` → online
     - Se `now - latest_handshake >= 180s` → offline
     - Se `latest_handshake == 0` → nunca conectou
   - Verificar se `endpoint` mudou (troca de IP do celular)
4. Atualizar state interno
5. Disponibilizar via `/metrics`

### 11.2 Health check dos proxies gost

**A cada 60 segundos:**
1. Para cada proxy ativo, tentar conectar via SOCKS5:
   ```
   CONNECT 1.1.1.1:443 via socks5://celX:PASS@127.0.0.1:108X
   ```
2. Se timeout (5s) ou connection refused → proxy com problema
3. Se o peer WireGuard está online mas o proxy falha → reiniciar gost
4. Se o peer está offline → proxy naturalmente falhará, não reiniciar

### 11.3 Detecção de queda

**Níveis de detecção:**

| Nível | O que falhou | Como detectar | Ação |
|---|---|---|---|
| 1 | Celular perdeu sinal | `latest_handshake` > 180s | Marcar offline, aguardar |
| 2 | App WireGuard morreu | `latest_handshake` > 300s + sem endpoint | Marcar offline, alerta |
| 3 | Proxy gost crashou | Health check SOCKS5 falha + peer online | Reiniciar gost |
| 4 | Routing quebrou | SOCKS5 conecta mas tráfego não flui | Recriar regras ip/iptables |
| 5 | WireGuard interface down | `wg show wg0` falha | Alerta crítico, `wg-quick up wg0` |

### 11.4 Logs

**Daemon:**
- Log estruturado (JSON) via `slog` (Go 1.21+)
- Rotação via `logrotate` ou log para `journald` (via systemd)
- Níveis: INFO (operações normais), WARN (peer offline), ERROR (falha de operação)

**Eventos logados:**
```json
{"level":"info","msg":"peer_added","device_id":1,"wg_ip":"10.0.0.2","time":"2025-01-15T14:30:00Z"}
{"level":"info","msg":"peer_online","device_id":1,"real_ip":"189.43.12.78","time":"2025-01-15T14:30:05Z"}
{"level":"warn","msg":"peer_offline","device_id":1,"last_handshake":"2025-01-15T14:27:00Z","time":"2025-01-15T14:30:15Z"}
{"level":"info","msg":"ip_changed","device_id":1,"old_ip":"189.43.12.78","new_ip":"189.43.12.99","time":"2025-01-15T15:00:00Z"}
{"level":"error","msg":"gost_crashed","device_id":1,"port":1081,"error":"signal: killed","time":"2025-01-15T15:30:00Z"}
{"level":"info","msg":"gost_restarted","device_id":1,"port":1081,"time":"2025-01-15T15:30:01Z"}
```

### 11.5 Alertas

O backend pode expor um endpoint de alertas ou integrar com:
- **n8n:** Webhook para alertas (device offline por mais de 10 minutos)
- **Telegram Bot:** Notificação direta
- **Webhook genérico:** Para integração com qualquer sistema

**Alertas recomendados:**
- Device offline por mais de 5 minutos
- Troca de IP (informativo)
- Gost crashou e foi reiniciado
- Nenhum device online (crítico)
- WireGuard interface down (crítico)

---

## 12. Escalabilidade

### 12.1 Limites por escala

| Escala | WireGuard | gost (RAM) | Policy routing | Portas | CPU | Status |
|---|---|---|---|---|---|---|
| 10 | Trivial | ~50 MB | 10 tabelas | 1081-1090 | <1% | ✅ Sem problemas |
| 50 | Fácil | ~250 MB | 50 tabelas | 1081-1130 | <5% | ✅ Confortável |
| 100 | OK | ~500 MB | 100 tabelas | 1081-1180 | <10% | ⚠️ Precisa organização |
| 253 | Limite /24 | ~1.3 GB | 253 tabelas | 1081-1334 | <20% | ⚠️ Próximo do limite |

### 12.2 Gargalos e soluções

**Gargalo 1: Muitos processos gost**
- Com 100+ devices, 100+ processos gost consomem RAM e file descriptors
- **Solução:** Substituir múltiplas instâncias gost por um proxy SOCKS5 customizado em Go que faz multiplexing interno. Um único processo que escuta em todas as portas e faz routing por porta usando `syscall.SetsockoptInt` para SO_MARK.
- **Implementação futura:** Não necessário até ~50 devices

**Gargalo 2: Tabelas de roteamento**
- 253 tabelas em `/etc/iproute2/rt_tables` é gerenciável
- O lookup por fwmark é O(n) em ip rules
- **Solução:** Usar números de tabela diretos sem nomes (elimina parsing do rt_tables)
- `ip rule add fwmark 101 table 101` (sem precisar mapear nome)

**Gargalo 3: Regras iptables**
- Muitas regras na chain mangle/OUTPUT impactam performance
- **Solução:** Usar nftables em vez de iptables (melhor performance com muitas regras)
- Ou usar ipsets para agrupar marks

**Gargalo 4: Subnet /24 limita a 253 devices**
- **Solução:** Usar /16 (10.0.0.0/16 → 65534 devices)
- Mudar apenas o Address do wg0 para 10.0.0.1/16 e ajustar AllowedIPs dos peers

### 12.3 Estratégias de expansão

**Para > 253 devices:**
1. Expandir subnet para /16
2. Implementar proxy multiplexer (um processo, múltiplas portas)
3. Migrar de iptables para nftables
4. Considerar múltiplas VPSs com load balancing

**Para alta disponibilidade:**
1. Segundo host WireGuard como standby
2. Floating IP entre hosts (keepalived)
3. Replicação do PostgreSQL
4. Celulares configurados com dois peers (primário + backup)

---

## 13. Boas Práticas de Produção

### 13.1 Segurança

- **Chaves privadas WireGuard:** Criptografadas no banco (AES-256-GCM), nunca em logs
- **Senhas dos proxies:** Criptografadas no banco
- **Unix socket:** Permissão 0660, grupo docker
- **Proxies SOCKS5:** Escutam em 127.0.0.1 (localhost only), não em 0.0.0.0
- **Dashboard:** Se acessível externamente, adicionar autenticação (JWT ou basic auth via nginx)
- **Firewall:** Apenas porta UDP 51820 aberta externamente; portas SOCKS5 (1081+) apenas em localhost
- **PresharedKey:** Sempre usar PSK além das chaves públicas (proteção contra ataques quânticos futuros)

### 13.2 Performance

- **MTU do WireGuard:** Testar e ajustar para evitar fragmentação. Default 1420. Operadoras 4G podem precisar 1380 ou 1340.
  - Teste: `ping -M do -s 1372 10.0.0.2` (pelo túnel). Se fragmentar, reduzir.
- **PersistentKeepalive:** 25 segundos é o sweet spot. Menor = mais tráfego desnecessário. Maior = risco do NAT da operadora expirar.
- **Polling interval do daemon:** 15 segundos é bom equilíbrio. Menor = mais CPU. Maior = detecção mais lenta.

### 13.3 Manutenção

- **Backup do PostgreSQL:** Cron job diário (`pg_dump`)
- **Cleanup de métricas antigas:** Job periódico para apagar `device_metrics` com mais de 90 dias
- **Atualização do gost:** Baixar nova versão, reiniciar instâncias uma a uma (o daemon gerencia)
- **Atualização do WireGuard:** `apt upgrade wireguard` — o kernel module atualiza sem downtime
- **Atualização do daemon:** Compilar novo binário, `systemctl restart wg-manager`. O daemon re-lê o estado do PostgreSQL e recria processos gost.

### 13.4 Idempotência

Todas as operações do daemon devem ser idempotentes:
- Adicionar peer que já existe → verificar antes, skip ou update
- Criar rota que já existe → verificar com `ip route show table X`, skip se existe
- Iniciar gost em porta já ocupada → verificar com `ss -tlnp`, skip se rodando
- Remover peer que não existe → não falhar, retornar success

Isso é crítico para o cenário de restart do daemon, onde ele precisa reconciliar o estado do PostgreSQL com o estado real do host.

### 13.5 Graceful shutdown

O daemon deve interceptar SIGTERM e SIGINT:
1. Parar goroutine de monitoramento
2. Parar todos os processos gost (SIGTERM → esperar 5s → SIGKILL)
3. **NÃO** remover peers do WireGuard (eles devem persistir para reconexão)
4. **NÃO** remover regras de routing (elas devem persistir para quando o daemon voltar)
5. Fechar Unix socket
6. Exit

No startup, o daemon reconcilia: verifica o que existe no host vs o que está no banco, e ajusta.

---

## 14. Checklist de Execução

### Fase 1 — Setup do Host (pré-requisitos)

- [ ] Instalar WireGuard (`apt install wireguard wireguard-tools`)
- [ ] Verificar kernel module (`modprobe wireguard && lsmod | grep wireguard`)
- [ ] Habilitar IP forwarding (`sysctl net.ipv4.ip_forward=1`, persistir em `/etc/sysctl.conf`)
- [ ] Abrir porta UDP 51820 no firewall
- [ ] Gerar keypair do servidor WireGuard
- [ ] Criar `/etc/wireguard/wg0.conf` (config base, sem peers)
- [ ] Subir interface: `wg-quick up wg0`
- [ ] Habilitar no boot: `systemctl enable wg-quick@wg0`
- [ ] Verificar: `wg show wg0` deve mostrar a interface ativa
- [ ] Configurar NAT: `iptables -t nat -A POSTROUTING -o wg0 -j MASQUERADE`
- [ ] Persistir regra NAT (via PostUp no wg0.conf ou iptables-persistent)
- [ ] Baixar binário `gost` v3 para `/usr/local/bin/gost`
- [ ] Verificar: `gost -V`
- [ ] Obter IP público da VPS: `curl -4 ifconfig.me` (anotar para configs dos celulares)

### Fase 2 — WG Manager Daemon

- [ ] Criar projeto Go em `daemon/`
- [ ] Implementar `internal/wireguard/keygen.go`: funções para gerar keypair e PSK via `exec.Command("wg", "genkey")` etc.
- [ ] Implementar `internal/wireguard/manager.go`: funções para `wg set` (add/remove peer), `wg show` (dump/parse)
- [ ] Implementar `internal/wireguard/config.go`: geração da string config .conf do celular
- [ ] Implementar `internal/routing/policy.go`: criação/remoção de tabelas de roteamento, ip rule, ip route
- [ ] Implementar `internal/routing/iptables.go`: criação/remoção de regras mangle/MARK por UID
- [ ] Implementar `internal/proxy/gost.go`: start/stop/status de processos gost (com user Linux dedicado)
- [ ] Implementar `internal/proxy/health.go`: health check SOCKS5 (dial + handshake)
- [ ] Implementar `internal/monitor/peers.go`: goroutine de polling `wg show` a cada 15s
- [ ] Implementar `internal/monitor/metrics.go`: cálculo de deltas rx/tx, detecção de status online/offline
- [ ] Implementar `internal/api/server.go`: HTTP server em Unix socket
- [ ] Implementar `internal/api/handlers.go`: handlers para POST /peers, DELETE /peers/{id}, GET /peers, GET /metrics, GET /health
- [ ] Implementar `cmd/wg-manager/main.go`: entrypoint, graceful shutdown, reconciliação no startup
- [ ] Compilar: `go build -o /usr/local/bin/wg-manager ./cmd/wg-manager/`
- [ ] Criar systemd unit `wg-manager.service`
- [ ] Habilitar e iniciar: `systemctl enable --now wg-manager`
- [ ] Testar: `curl --unix-socket /var/run/wg-manager.sock http://localhost/health`

### Fase 3 — PostgreSQL (Schema)

- [ ] Criar database `proxy_manager` no PostgreSQL existente
- [ ] Executar migration 001: tabela `devices`
- [ ] Executar migration 002: tabela `device_metrics`
- [ ] Executar migration 003: tabela `device_events`
- [ ] Verificar: `\dt` no psql mostra as 3 tabelas

### Fase 4 — Backend API

- [ ] Criar projeto Go em `backend/`
- [ ] Implementar `internal/daemon/client.go`: HTTP client via Unix socket para o daemon
- [ ] Implementar `internal/db/postgres.go`: conexão e pool (pgx ou database/sql)
- [ ] Implementar `internal/db/migrations.go`: auto-migration no startup
- [ ] Implementar `internal/db/queries.go`: CRUD devices, insert metrics, insert events
- [ ] Implementar `internal/models/`: structs Device, Proxy, Metrics, Event
- [ ] Implementar `internal/handlers/devices.go`: GET/POST/DELETE /api/devices
- [ ] Implementar `internal/handlers/proxies.go`: GET /api/proxies, GET /api/proxies/available
- [ ] Implementar `internal/handlers/qrcode.go`: geração de QR Code com `skip2/go-qrcode`
- [ ] Implementar `internal/handlers/metrics.go`: GET /api/metrics, GET /api/metrics/{id}
- [ ] Implementar `internal/websocket/hub.go`: WebSocket hub para broadcast de eventos
- [ ] Implementar polling loop: a cada 15s, chamar daemon /metrics → calcular deltas → update DB → emit WS
- [ ] Implementar `internal/api/server.go`: HTTP server :8080 com routing (chi ou gorilla/mux)
- [ ] Implementar `internal/api/middleware.go`: CORS, logging, recovery
- [ ] Implementar `cmd/api/main.go`: entrypoint, graceful shutdown
- [ ] Criar `Dockerfile` (multi-stage: Go build → scratch/alpine)
- [ ] Testar localmente: `go run ./cmd/api/` + `curl http://localhost:8080/api/devices`

### Fase 5 — Dashboard

- [ ] Criar projeto React em `dashboard/`
- [ ] Setup Vite + React + Tailwind
- [ ] Implementar `services/api.js`: client HTTP para o backend
- [ ] Implementar `hooks/useWebSocket.js`: conexão WS com reconnect
- [ ] Implementar `hooks/useDevices.js`: state management de devices
- [ ] Implementar `components/DeviceList.jsx`: lista de devices com status
- [ ] Implementar `components/DeviceCard.jsx`: card com métricas, proxy info, ações
- [ ] Implementar `components/AddDeviceModal.jsx`: formulário + QR Code + status live
- [ ] Implementar `components/StatusBadge.jsx`: badge online/offline
- [ ] Implementar `components/ProxyInfo.jsx`: info do proxy com copy-to-clipboard
- [ ] Implementar `components/TrafficChart.jsx`: gráfico recharts de tráfego
- [ ] Implementar `App.jsx`: layout principal, routing entre telas
- [ ] Criar `Dockerfile` (multi-stage: node build → nginx)
- [ ] Criar `nginx.conf` com proxy_pass para backend API e WebSocket
- [ ] Build e testar: `npm run build` + verificar output em `dist/`

### Fase 6 — Docker Swarm Stack

- [ ] Criar `deploy/docker-compose.yml` com serviços: backend, dashboard, postgres
- [ ] Configurar volumes: pgdata, wg-manager.sock bind mount
- [ ] Configurar rede overlay: proxy-manager-net (attachable)
- [ ] Configurar variáveis de ambiente: DB_URL, ENCRYPTION_KEY, VPS_PUBLIC_IP
- [ ] Deploy via Portainer ou `docker stack deploy -c docker-compose.yml proxy-manager`
- [ ] Verificar: `docker service ls` mostra os 3 serviços running
- [ ] Testar: `curl http://192.168.100.152:8080/api/devices` retorna `[]`
- [ ] Testar: `curl http://192.168.100.152:8000` serve o dashboard

### Fase 7 — Teste end-to-end

- [ ] Acessar dashboard em `http://192.168.100.152:8000`
- [ ] Clicar "Adicionar dispositivo"
- [ ] Inserir nome, clicar "Gerar"
- [ ] Verificar QR Code exibido
- [ ] Escanear QR Code no app WireGuard do celular
- [ ] Verificar no dashboard: status muda para "Online"
- [ ] Verificar no host: `wg show wg0` mostra o peer com handshake recente
- [ ] Verificar no host: `ss -tlnp | grep 1081` mostra gost escutando
- [ ] Testar proxy: `curl --socks5-hostname cel1:SENHA@127.0.0.1:1081 https://ifconfig.me` → deve retornar IP do celular (4G)
- [ ] Se retornar IP da VPS em vez do celular → problema no routing, verificar iptables/ip rule

### Fase 8 — Integração com GoWA/WhatsMeow

- [ ] Modificar GoWA para aceitar configuração de proxy SOCKS5 (variável de ambiente ou via API)
- [ ] Configurar instância GoWA com `PROXY_URL=socks5://cel1:SENHA@127.0.0.1:1081`
- [ ] Testar: WhatsMeow conecta via proxy → verificar no dashboard que tráfego aumenta
- [ ] Implementar seleção dinâmica de proxy (consultar backend API)
- [ ] Implementar failover: se proxy primário falha, buscar alternativo

### Fase 9 — Monitoramento e alertas

- [ ] Verificar que o daemon está logando em journald: `journalctl -u wg-manager -f`
- [ ] Verificar que eventos estão sendo gravados na tabela `device_events`
- [ ] Verificar que métricas históricas estão na tabela `device_metrics`
- [ ] Configurar alerta (webhook n8n ou Telegram) para device offline > 5min
- [ ] Testar: desligar dados móveis do celular → verificar alerta em < 3min
- [ ] Testar: ligar dados móveis → verificar que status volta para online automaticamente
- [ ] Testar: matar processo gost → verificar que daemon reinicia em < 60s

### Fase 10 — Hardening de produção

- [ ] Verificar que proxies escutam apenas em 127.0.0.1 (não 0.0.0.0)
- [ ] Verificar que Unix socket tem permissão 0660
- [ ] Configurar backup diário do PostgreSQL
- [ ] Configurar cleanup de métricas antigas (> 90 dias)
- [ ] Configurar logrotate para logs do daemon
- [ ] Testar MTU: `ping -M do -s 1372 10.0.0.2` — ajustar se necessário
- [ ] Documentar IP público da VPS e anotar configuração base
- [ ] Testar reinício completo do host: `reboot` → verificar que tudo sobe automaticamente

---

## Apêndice A — Referência de Comandos WireGuard

```bash
# Gerar keypair
wg genkey | tee privatekey | wg pubkey > publickey

# Gerar preshared key
wg genpsk > presharedkey

# Adicionar peer (hot reload, sem restart)
wg set wg0 peer <PUBKEY> preshared-key /tmp/pskfile allowed-ips 10.0.0.X/32

# Remover peer
wg set wg0 peer <PUBKEY> remove

# Listar peers e status
wg show wg0

# Dump (machine-readable)
wg show wg0 dump

# Verificar handshakes
wg show wg0 latest-handshakes

# Verificar transferência
wg show wg0 transfer
```

## Apêndice B — Referência de Policy Routing

```bash
# Criar tabela nomeada (opcional)
echo "101 cel1" >> /etc/iproute2/rt_tables

# Criar rota na tabela
ip route add default dev wg0 via 10.0.0.2 table 101

# Criar regra: fwmark → tabela
ip rule add fwmark 101 table 101

# Marcar pacotes por UID do processo
iptables -t mangle -A OUTPUT -m owner --uid-owner gost-cel1 -j MARK --set-mark 101

# Verificar regras
ip rule show
ip route show table 101
iptables -t mangle -L OUTPUT -v -n

# Remover (ordem inversa)
iptables -t mangle -D OUTPUT -m owner --uid-owner gost-cel1 -j MARK --set-mark 101
ip rule del fwmark 101 table 101
ip route del default table 101
```

## Apêndice C — Referência do gost v3

```bash
# SOCKS5 com autenticação
gost -L "socks5://user:pass@:1081"

# Verificar se está rodando
ss -tlnp | grep 1081

# Testar proxy
curl --socks5-hostname user:pass@127.0.0.1:1081 https://ifconfig.me
```

## Apêndice D — Formato do `wg show wg0 dump`

```
# Primeira linha: interface
# private_key  public_key  listen_port  fwmark

# Linhas seguintes: peers (tab-separated)
# public_key  preshared_key  endpoint  allowed_ips  latest_handshake  rx_bytes  tx_bytes  persistent_keepalive

# Exemplo:
abc123...  (none)  189.43.12.78:34567  10.0.0.2/32  1705312200  1048576  524288  25
```

Campos:
- `public_key`: base64 encoded (44 chars)
- `preshared_key`: base64 ou `(none)`
- `endpoint`: IP:porta real do peer (vazio se nunca conectou)
- `allowed_ips`: CIDR do peer
- `latest_handshake`: Unix timestamp (0 = nunca)
- `rx_bytes`: bytes recebidos do peer (acumulativo)
- `tx_bytes`: bytes enviados ao peer (acumulativo)
- `persistent_keepalive`: intervalo em segundos ou `off`
