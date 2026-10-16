# Binary RPM built from the artifacts produced by `wails build`.
# Used by scripts/package-linux.sh; not meant for Koji/COPR as-is.
%global debug_package %{nil}

Name:           anansi
Version:        %{app_version}
Release:        1%{?dist}
Summary:        Gerenciador desktop de bancos de dados MySQL e MariaDB
License:        Proprietary
URL:            https://localhost/anansi
Source0:        anansi
Source1:        anansi.desktop
Source2:        anansi.png

Requires:       gtk3
Requires:       webkit2gtk4.1
# Passwords are stored through the Secret Service D-Bus API.
Recommends:     gnome-keyring

%description
Anansi é um gerenciador de bancos de dados desktop com múltiplas
conexões, abas SQL independentes e autocomplete baseado nos metadados.

%prep

%build

%install
install -Dm0755 %{SOURCE0} %{buildroot}%{_bindir}/anansi
install -Dm0644 %{SOURCE1} %{buildroot}%{_datadir}/applications/anansi.desktop
install -Dm0644 %{SOURCE2} %{buildroot}%{_datadir}/icons/hicolor/512x512/apps/anansi.png

%files
%{_bindir}/anansi
%{_datadir}/applications/anansi.desktop
%{_datadir}/icons/hicolor/512x512/apps/anansi.png

%changelog
* Wed Oct 14 2026 Anansi <dev@localhost> - 0.1.0-1
- Primeira versão: MySQL/MariaDB, múltiplas conexões, abas SQL e autocomplete.
