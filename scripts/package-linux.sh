#!/usr/bin/env bash
# Builds the desktop binary and produces distributable packages in dist/:
#   - anansi-<version>-linux-amd64.tar.gz (binary, .desktop, icon, install.sh)
#   - anansi-<version>-1.<dist>.x86_64.rpm (when rpmbuild is available)
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION=$(sed -n 's/^const Version = "\(.*\)"/\1/p' internal/api/app.go)
WAILS=${WAILS:-$(command -v wails || echo "$HOME/go/bin/wails")}

"$WAILS" build -clean -trimpath -tags webkit2_41 -ldflags "-s -w"
BIN=build/bin/anansi
mkdir -p dist

# tar.gz
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
PKG="$STAGE/anansi-$VERSION"
install -Dm0755 "$BIN" "$PKG/anansi"
install -Dm0644 packaging/linux/anansi.desktop "$PKG/anansi.desktop"
install -Dm0644 build/appicon.png "$PKG/anansi.png"
cat > "$PKG/install.sh" <<'INSTALL'
#!/usr/bin/env bash
# Installs Anansi for the current user (~/.local).
set -euo pipefail
cd "$(dirname "$0")"
install -Dm0755 anansi "$HOME/.local/bin/anansi"
install -Dm0644 anansi.desktop "$HOME/.local/share/applications/anansi.desktop"
install -Dm0644 anansi.png "$HOME/.local/share/icons/hicolor/512x512/apps/anansi.png"
echo "Instalado em ~/.local/bin/anansi"
INSTALL
chmod +x "$PKG/install.sh"
tar -C "$STAGE" -czf "dist/anansi-$VERSION-linux-amd64.tar.gz" "anansi-$VERSION"
echo "dist/anansi-$VERSION-linux-amd64.tar.gz"

# RPM
if command -v rpmbuild >/dev/null; then
  TOP=$(mktemp -d)
  mkdir -p "$TOP"/{SOURCES,SPECS,BUILD,RPMS,SRPMS}
  cp "$BIN" packaging/linux/anansi.desktop "$TOP/SOURCES/"
  cp build/appicon.png "$TOP/SOURCES/anansi.png"
  rpmbuild --quiet -bb --define "_topdir $TOP" --define "app_version $VERSION" packaging/linux/anansi.spec
  find "$TOP/RPMS" -name '*.rpm' -exec cp {} dist/ \;
  rm -rf "$TOP"
  ls dist/*.rpm
else
  echo "rpmbuild não encontrado: pacote RPM não gerado (sudo dnf install rpm-build)"
fi
