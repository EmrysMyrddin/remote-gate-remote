#!/bin/bash
set -e

if [[ -z "$1" ]]; then
  echo "Please provide a version number"
  false
fi

echo "#define VERSION \"$1\"" > version.h
arduino-cli compile --fqbn esp32:esp32:esp32 gate.ino -v --output-dir build
mv build/gate.ino.bin build/$1.bin