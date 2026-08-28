#!/usr/bin/env bash
# Construit gh-cohorte pour toutes les plateformes prises en charge.
# Appelé par le workflow de publication avec le tag en premier argument ;
# utilisable aussi à la main : script/build.sh v1.0.0
set -euo pipefail

nom="gh-cohorte"
tag="${1:-dev}"
sortie="dist"

# linux, macOS et Windows, en 64 bits Intel comme ARM.
plateformes=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

rm -rf "$sortie"
mkdir -p "$sortie"

for plateforme in "${plateformes[@]}"; do
  read -r os arch <<< "$plateforme"
  fichier="$sortie/${nom}_${tag}_${os}-${arch}"
  if [ "$os" = "windows" ]; then
    fichier="${fichier}.exe"
  fi
  echo "→ $fichier"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=${tag}" -o "$fichier" .
done

echo "Binaires produits dans $sortie/"
