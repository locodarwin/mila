#!/bin/sh
# Makes the .deb and .rpm of one Linux program (make release-bins), with
# dpkg-deb and rpmbuild: the program, the unit made from deploy/mila.service,
# the settings file, the drop-in for Mila's own HTTPS, the license, and the
# scripts here.
#   sh deploy/package/build.sh <program> <amd64|arm64> <version> <out dir>
set -eu
program=$1 arch=$2 version=${3#v} out=$4
here=$(cd "$(dirname "$0")" && pwd)
out=$(cd "$out" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

summary='A project manager for a person or a small team'
about='One program with the app, the server and its database: tasks, projects,
views, a wiki, time tracking and reports, kept in sync on every device.'
maintainer='locodarwin <locodarwin@users.noreply.github.com>'
homepage=https://github.com/locodarwin/mila

# A version both can order: 2.8.0 for a release, 2.8.0+5.g10947da for a build
# after it, which sorts after 2.8.0 and before 2.8.1.
pkgversion=$(printf '%s' "$version" | sed 's/-/+/; s/-/./g')
case $arch in
amd64) rpmarch=x86_64 ;;
arm64) rpmarch=aarch64 ;;
*) echo "build.sh: no packages for $arch" >&2; exit 1 ;;
esac

# The files, as installed.
root=$stage/root
install -D -m 755 "$program" "$root/usr/bin/mila"
install -D -m 640 "$here/mila.toml" "$root/etc/mila/mila.toml"
install -D -m 644 "$here/own-https.conf" "$root/usr/share/mila/own-https.conf"
install -D -m 644 "$here/../../LICENSE" "$root/usr/share/doc/mila/LICENSE"
mkdir -p "$root/usr/lib/systemd/system"
# The unit, as deploy/mila.service has it but for where a package puts the
# program, and its settings in /etc/mila/mila.toml rather than in the unit.
{
	echo "# Mila's systemd unit, as the .deb and .rpm packages install it. Its"
	echo "# settings are in /etc/mila/mila.toml; a drop-in, sudo systemctl edit mila,"
	echo "# overrides the unit, and its Environment= lines the file."
	sed -n '/^\[Unit\]/,$p' "$here/../mila.service" |
		sed -e 's|^ExecStart=.*|ExecStart=/usr/bin/mila serve|' \
			-e '/^Environment=MILA_LISTEN=/d' \
			-e 's|^Environment=MILA_BASE_URL=.*|Environment=MILA_CONFIG=/etc/mila/mila.toml|'
} >"$root/usr/lib/systemd/system/mila.service"
chmod 644 "$root/usr/lib/systemd/system/mila.service"
grep -q '^ExecStart=/usr/bin/mila serve$' "$root/usr/lib/systemd/system/mila.service"
grep -q '^Environment=MILA_CONFIG=/etc/mila/mila.toml$' "$root/usr/lib/systemd/system/mila.service"

# The .deb.
deb=$stage/deb
cp -a "$root" "$deb"
mkdir -p "$deb/DEBIAN"
{
	echo "Package: mila"
	echo "Version: $pkgversion-1"
	echo "Architecture: $arch"
	echo "Maintainer: $maintainer"
	echo "Section: net"
	echo "Priority: optional"
	echo "Homepage: $homepage"
	echo "Installed-Size: $(du -sk "$root" | cut -f1)"
	echo "Description: $summary"
	printf '%s\n' "$about" | sed 's/^/ /'
} >"$deb/DEBIAN/control"
echo /etc/mila/mila.toml >"$deb/DEBIAN/conffiles"
install -m 755 "$here/postinstall.sh" "$deb/DEBIAN/postinst"
install -m 755 "$here/preremove.sh" "$deb/DEBIAN/prerm"
install -m 755 "$here/postremove.sh" "$deb/DEBIAN/postrm"
dpkg-deb --root-owner-group -Zxz --build "$deb" "$out/mila_${pkgversion}-1_${arch}.deb" >/dev/null

# The .rpm, of the same files, built for its architecture with no compiling.
rpm=$stage/rpm
mkdir -p "$rpm/SPECS"
{
	cat <<SPEC
Name: mila
Version: $pkgversion
Release: 1
Summary: $summary
License: MIT
URL: $homepage
Packager: $maintainer
%global debug_package %{nil}
%global __os_install_post %{nil}
%global _build_id_links none
%global source_date_epoch_from_changelog 0

%description
$about

%install
cp -a "$root/." "%{buildroot}/"

%files
%attr(0755,root,root) /usr/bin/mila
%attr(0644,root,root) /usr/lib/systemd/system/mila.service
%dir /etc/mila
%config(noreplace) %attr(0640,root,root) /etc/mila/mila.toml
%dir /usr/share/mila
%attr(0644,root,root) /usr/share/mila/own-https.conf
%dir /usr/share/doc/mila
%license %attr(0644,root,root) /usr/share/doc/mila/LICENSE

SPEC
	echo "%post"
	cat "$here/postinstall.sh"
	echo
	echo "%preun"
	cat "$here/preremove.sh"
	echo
	echo "%postun"
	cat "$here/postremove.sh"
} >"$rpm/SPECS/mila.spec"
rpmbuild --quiet -bb --target "$rpmarch" --define "_topdir $rpm" "$rpm/SPECS/mila.spec"
cp "$rpm/RPMS/$rpmarch/"mila-*.rpm "$out/"
