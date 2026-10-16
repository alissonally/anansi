#!/usr/bin/env bash
# Builds the desktop binary and produces distributable packages in dist/:
#   - dbstudio-<version>-linux-amd64.tar.gz (binary, .desktop, icon, install.sh)
#   - dbstudio-<version>-1.<dist>.x86_64.rpm (when rpmbuild is available)
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION=$(sed -n 's/^const Version = "\(.*\)"/\1/p' internal/api/app.go)
WAILS=${WAILS:-$(command -v wails || echo "$HOME/go/bin/wails")}

"$WAILS" build -clean -trimpath -tags webkit2_41 -ldflags "-s -w"
BIN=build/bin/dbstudio
mkdir -p dist

# tar.gz
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
PKG="$STAGE/dbstudio-$VERSION"
install -Dm0755 "$BIN" "$PKG/dbstudio"
install -Dm0644 packaging/linux/dbstudio.desktop "$PKG/dbstudio.desktop"
install -Dm0644 build/appicon.png "$PKG/dbstudio.png"
cat > "$PKG/install.sh" <<'INSTALL'
#!/usr/bin/env bash
# Installs Database Studio for the current user (~/.local).
set -euo pipefail
cd "$(dirname "$0")"
install -Dm0755 dbstudio "$HOME/.local/bin/dbstudio"
install -Dm0644 dbstudio.desktop "$HOME/.local/share/applications/dbstudio.desktop"
install -Dm0644 dbstudio.png "$HOME/.local/share/icons/hicolor/512x512/apps/dbstudio.png"
echo "Instalado em ~/.local/bin/dbstudio"
INSTALL
chmod +x "$PKG/install.sh"
tar -C "$STAGE" -czf "dist/dbstudio-$VERSION-linux-amd64.tar.gz" "dbstudio-$VERSION"
echo "dist/dbstudio-$VERSION-linux-amd64.tar.gz"

# RPM
if command -v rpmbuild >/dev/null; then
  TOP=$(mktemp -d)
  mkdir -p "$TOP"/{SOURCES,SPECS,BUILD,RPMS,SRPMS}
  cp "$BIN" packaging/linux/dbstudio.desktop "$TOP/SOURCES/"
  cp build/appicon.png "$TOP/SOURCES/dbstudio.png"
  rpmbuild --quiet -bb --define "_topdir $TOP" --define "app_version $VERSION" packaging/linux/dbstudio.spec
  find "$TOP/RPMS" -name '*.rpm' -exec cp {} dist/ \;
  rm -rf "$TOP"
  ls dist/*.rpm
else
  echo "rpmbuild não encontrado: pacote RPM não gerado (sudo dnf install rpm-build)"
fi
