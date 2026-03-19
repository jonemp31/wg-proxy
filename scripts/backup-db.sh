#!/bin/bash
set -euo pipefail

# Backup do PostgreSQL do Proxy Manager
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/var/backups/proxy-manager"
mkdir -p "$BACKUP_DIR"

# Encontrar o container do postgres
CONTAINER=$(docker ps --filter "name=proxy-manager_postgres" --format "{{.ID}}" | head -1)

if [ -z "$CONTAINER" ]; then
    echo "ERRO: Container PostgreSQL não encontrado"
    exit 1
fi

docker exec "$CONTAINER" pg_dump -U proxy_manager proxy_manager | gzip > "$BACKUP_DIR/proxy_manager_${TIMESTAMP}.sql.gz"

# Manter apenas últimos 30 backups
ls -tp "$BACKUP_DIR"/*.sql.gz 2>/dev/null | tail -n +31 | xargs -r rm --

echo "Backup salvo: $BACKUP_DIR/proxy_manager_${TIMESTAMP}.sql.gz"
