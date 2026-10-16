#!/usr/bin/env bash
# Starts/stops disposable MySQL and MariaDB servers for integration tests
# using podman (preferred on Fedora) or docker.
#   scripts/integration-db.sh up|down|env
set -euo pipefail
ENGINE=${CONTAINER_ENGINE:-$(command -v podman || command -v docker || true)}
[ -n "$ENGINE" ] || { echo "podman ou docker é necessário para os testes de integração" >&2; exit 1; }
PW=studio-test-pw
case "${1:-}" in
  up)
    "$ENGINE" rm -f anansi-it-mysql anansi-it-mariadb >/dev/null 2>&1 || true
    "$ENGINE" run -d --name anansi-it-mysql -p 127.0.0.1:13306:3306 --tmpfs /var/lib/mysql \
      -e MYSQL_ROOT_PASSWORD=$PW -e MYSQL_DATABASE=studio_it docker.io/library/mysql:8.4 >/dev/null
    "$ENGINE" run -d --name anansi-it-mariadb -p 127.0.0.1:13307:3306 --tmpfs /var/lib/mysql \
      -e MARIADB_ROOT_PASSWORD=$PW -e MARIADB_DATABASE=studio_it docker.io/library/mariadb:11.4 >/dev/null
    echo "aguardando os servidores..."
    for i in $(seq 1 90); do
      ok=0
      "$ENGINE" exec anansi-it-mysql mysql -uroot -p$PW -h127.0.0.1 -e 'SELECT 1' >/dev/null 2>&1 && ok=$((ok+1))
      "$ENGINE" exec anansi-it-mariadb mariadb -uroot -p$PW -h127.0.0.1 -e 'SELECT 1' >/dev/null 2>&1 && ok=$((ok+1))
      [ $ok -eq 2 ] && { echo "prontos"; exit 0; }
      sleep 2
    done
    echo "tempo esgotado aguardando os servidores" >&2; exit 1 ;;
  down)
    "$ENGINE" rm -f anansi-it-mysql anansi-it-mariadb >/dev/null ;;
  env)
    echo "export ANANSI_IT_MYSQL=127.0.0.1:13306 ANANSI_IT_MARIADB=127.0.0.1:13307 ANANSI_IT_PASSWORD=$PW" ;;
  *) echo "uso: $0 up|down|env" >&2; exit 2 ;;
esac
