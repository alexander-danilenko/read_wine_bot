#!/bin/sh
# Creates the bot's own login and database on the first start of an empty volume. psql quotes
# the :"name" and :'value' variables, so names and passwords need no escaping here.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" \
	-v user="$DB_USER" -v password="$DB_PASSWORD" -v db="$DB_NAME" <<'SQL'
CREATE ROLE :"user" LOGIN PASSWORD :'password';
CREATE DATABASE :"db" OWNER :"user";
SQL
