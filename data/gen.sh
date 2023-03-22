#!/bin/bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
REPO_DIR="$SCRIPT_DIR/.."
TMP_DIR="$REPO_DIR/tmp5"

python3 import.py

rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"
cd "$TMP_DIR"
dolt init
dolt sql < "$SCRIPT_DIR/plants.sql"
dolt sql < "$SCRIPT_DIR/animals.sql"
dolt add plants animals
dolt commit -m "add plants and animals"
