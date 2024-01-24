#!/bin/bash


track_table() {
  local db_name=$1
  local schema_name=$2
  local table_name=$3

  curl --silent --fail -X POST http://localhost:8080/v1/metadata \
       -H "Content-Type: application/json" \
       -H "X-Hasura-Admin-Secret: your-admin-secret" \
       --data-raw '{
         "type": "pg_track_table",
         "args": {
           "source": "'"${db_name}"'",
           "schema": "'"${schema_name}"'",
           "name": "'"${table_name}"'"
         }
       }'
}

sleep 6

graphql-engine serve &
pid=$!

sleep 20

echo "Hasura is up. Tracking tables..."

track_table "postgres" "public" "Regions"
track_table "postgres" "public" "Projects"

echo "Tables tracked successfully."

wait $pid