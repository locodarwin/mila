#!/bin/sh
# Installs Mila on Linux with systemd, upgrades it, or removes it.
#
#   sudo sh install.sh                 install, or upgrade what is installed
#   sudo sh install.sh --uninstall     remove Mila, keeping its data
#   sudo sh install.sh --uninstall --purge   remove Mila and its data
#
# Read it before running it. What it does:
#   1. Downloads the server program for this machine from Mila's latest
#      release (or --version's), with SHA256SUMS, and checks it; or takes
#      both from a folder you downloaded them to (--from, with --version).
#      From 3.0, with gh installed, it also checks GitHub's attestation that
#      Mila's release workflow built the program from that release's tag (no
#      sign-in needed); without gh, it says that only the checksum was
#      checked. It never replaces Mila with an older release unless
#      --version names that release.
#   2. Installs the program as /usr/local/bin/mila and the systemd unit as
#      /etc/systemd/system/mila.service, and makes the system user mila.
#   3. On a first install, asks for the domain, and whether Mila gets its own
#      HTTPS certificate (ports 80 and 443) or runs behind a proxy you have
#      (on 127.0.0.1:8080); the answers go in a drop-in beside the unit,
#      /etc/systemd/system/mila.service.d/, which upgrades leave alone.
#   4. Starts Mila and prints the link that makes the first account.
# The data, everything Mila keeps, is in /var/lib/mila.
#
# Options:
#   --domain NAME    the domain people use, such as mila.example.com
#   --email ADDRESS  for Let's Encrypt's notices about the certificate
#   --proxy          run behind a proxy you have, not with Mila's own HTTPS
#   --version vX.Y.Z install that release, not the latest, even if it is
#                    older than the Mila installed
#   --from DIR       take mila_<version>_linux_<arch>.tar.gz, SHA256SUMS and,
#                    from 3.0, mila.intoto.jsonl from DIR, for the release
#                    --version names (download them with: gh release download
#                    vX.Y.Z --repo locodarwin/mila --pattern
#                    'mila_*_linux_*.tar.gz' --pattern SHA256SUMS --pattern
#                    mila.intoto.jsonl)
#   --yes            ask nothing: --domain is then needed on a first install
#   --uninstall      stop and remove Mila; --purge also deletes its data and user
set -eu

REPO=locodarwin/mila
BIN=/usr/local/bin/mila
UNIT=/etc/systemd/system/mila.service
DROPINS=/etc/systemd/system/mila.service.d
DATA=/var/lib/mila
# Settings, passwords among them, readable by root and the mila group alone.
# Not in a drop-in's Environment= lines: systemd shows those to anyone.
CONF=/etc/mila/mila.toml

domain='' email='' proxy='' version='' from='' yes='' uninstall='' purge=''

say() { printf '%s\n' "$*"; }
fail() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

while [ $# -gt 0 ]; do
	case $1 in
	--domain) domain=${2-}; shift ;;
	--email) email=${2-}; shift ;;
	--proxy) proxy=yes ;;
	--version) version=${2-}; shift ;;
	--from) from=${2-}; shift ;;
	--yes) yes=yes ;;
	--uninstall) uninstall=yes ;;
	--purge) purge=yes ;;
	-h | --help)
		sed -n '/^set -eu$/q; 2,$p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*) fail "unknown option $1; --help lists them" ;;
	esac
	shift
done

[ "$(id -u)" = 0 ] || fail "run it as root, with sudo"
if ! [ -d /run/systemd/system ] || ! command -v systemctl >/dev/null; then
	fail "this computer does not run systemd; the guide has the other ways to run Mila"
fi
[ -z "$purge" ] || [ -n "$uninstall" ] || fail "--purge goes with --uninstall"

# Whether there is a terminal to ask on. /dev/tty is there even without one,
# so it has to be opened to tell.
has_tty() { (: </dev/tty) 2>/dev/null; }

# Asks on the terminal, even when the script comes through a pipe.
ask() { # question default
	if [ -n "$yes" ] || ! has_tty; then
		printf '%s\n' "$2"
		return
	fi
	printf '%s ' "$1" >/dev/tty
	read -r answer </dev/tty || answer=''
	printf '%s\n' "${answer:-$2}"
}

# Whether the user mila is one an install made: a system account that cannot
# sign in. A person's own account of that name is left alone.
system_mila() {
	id mila >/dev/null 2>&1 || return 1
	uid=$(id -u mila)
	max=$(sed -n 's/^SYS_UID_MAX[[:space:]]\{1,\}\([0-9]\{1,\}\).*/\1/p' /etc/login.defs 2>/dev/null | tail -n 1)
	[ "$uid" -lt "${max:-999}" ] || [ "$uid" -eq "${max:-999}" ] || return 1
	shell=$(getent passwd mila | cut -d: -f7)
	case $shell in */nologin | */false) return 0 ;; esac
	return 1
}

# Uninstalling.

if [ -n "$uninstall" ]; then
	if [ -n "$purge" ] && [ -z "$yes" ]; then
		a=$(ask "This deletes $DATA, every task, page and file Mila holds, and its backups. Type delete to go on:" '')
		[ "$a" = delete ] || fail "nothing removed"
	fi
	systemctl disable --now mila 2>/dev/null || true
	rm -f "$UNIT" "$BIN"
	rm -rf "$DROPINS"
	systemctl daemon-reload
	if [ -n "$purge" ]; then
		rm -rf "$DATA"
		rm -f "$CONF"
		rmdir /etc/mila 2>/dev/null || true
		if system_mila; then userdel mila; else say "The user mila is not a system account, so it is kept."; fi
		say "Mila and its data are removed."
	else
		say "Mila is removed. Its data is still in $DATA, its settings in $CONF, and"
		say "the user mila is kept; installing again takes them up where they were, the"
		say "settings as they are. --uninstall --purge deletes them."
	fi
	exit 0
fi

# Downloading and checking.

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "there is no Mila for $(uname -m) machines; Linux on amd64 or arm64 only" ;;
esac
command -v sha256sum >/dev/null || fail "sha256sum is needed to check the download"
command -v tar >/dev/null || fail "tar is needed to unpack the download"

# Whether a value is a version X.Y.Z, such as 3.0.0.
is_version() {
	case $1 in '' | *[!0-9.]*) return 1 ;; esac
	printf '%s\n' "$1" | grep -Eqx '(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
}

# Whether version $1 is older than version $2, both X.Y.Z.
older() {
	a=$1 b=$2
	for _ in 1 2 3; do
		x=${a%%.*} y=${b%%.*}
		[ "$x" -lt "$y" ] && return 0
		[ "$x" -gt "$y" ] && return 1
		a=${a#*.} b=${b#*.}
	done
	return 1
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fetch() { # url file
	if command -v curl >/dev/null; then
		curl -fsSL --proto '=https' -o "$2" "$1"
	elif command -v wget >/dev/null; then
		wget -q --https-only -O "$2" "$1"
	else
		fail "curl or wget is needed to download Mila, or --from a folder you downloaded it to"
	fi
}

# The tag of the latest release, as GitHub says: where it sends the latest
# release's page, or with wget alone (whose kinds tell redirects differently),
# its API's answer.
latest_tag() {
	if command -v curl >/dev/null; then
		to=$(curl -sS --proto '=https' -o /dev/null -w '%{redirect_url}' "https://github.com/$REPO/releases/latest") || to=''
		case $to in
		"https://github.com/$REPO/releases/tag/"?*) printf '%s\n' "${to#"https://github.com/$REPO/releases/tag/"}" ;;
		esac
	elif command -v wget >/dev/null; then
		fetch "https://api.github.com/repos/$REPO/releases/latest" "$work/latest.json" 2>/dev/null || return 0
		# Each of , { and } becomes a line break, so each field is a line.
		# shellcheck disable=SC2020
		tr ',{}' '\n\n\n' <"$work/latest.json" |
			sed -n 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)"[[:space:]]*$/\1/p' | head -n 1
	fi
}

# Which release, from what was asked for, never from the release's own files,
# which are what is being checked: --version's, or the one GitHub names as the
# latest. The program's name is then that release's, exactly.
explicit=''
if [ -n "$version" ]; then
	explicit=yes
	version=v${version#v}
	is_version "${version#v}" || fail "--version $version is not a release, such as v3.0.0"
elif [ -n "$from" ]; then
	fail "--from needs --version, the release the files in $from are from, such as --version v3.0.0"
else
	version=$(latest_tag)
	[ -n "$version" ] || fail "could not find Mila's latest release on GitHub; while the repository is private, use --from with --version"
	case $version in
	v*) is_version "${version#v}" || version='' ;;
	*) version='' ;;
	esac
	[ -n "$version" ] || fail "the latest release's tag is not a release such as v3.0.0, so nothing is installed"
fi
want=${version#v}
name=mila_${want}_linux_${arch}.tar.gz

# Attestations began with 3.0. An older release is installed only when asked
# for by name, and then with its checksum alone.
attested=yes
if older "$want" 3.0.0; then
	[ -n "$explicit" ] ||
		fail "the latest release, $version, is from before releases were attested; --version $version installs it all the same, checked by its checksum alone"
	attested=''
fi

# The Mila installed now, if any, is not replaced by an older one unless that
# is asked for. (This runs the program installed, root's own; the one
# downloaded is never run here.)
if [ -x "$BIN" ]; then
	installed=$("$BIN" version 2>/dev/null | sed -n '1s/^mila v\{0,1\}\([0-9][0-9.]*[0-9]\).*/\1/p' || true)
	if is_version "$installed" && older "$want" "$installed"; then
		[ -n "$explicit" ] ||
			fail "the latest release, $version, is older than the Mila installed, $installed, so nothing is installed; --version $version installs it all the same"
		say "Installing $version, older than the Mila installed ($installed), as asked."
	fi
fi

if [ -n "$from" ]; then
	[ -f "$from/SHA256SUMS" ] || fail "$from has no SHA256SUMS"
	cp "$from/SHA256SUMS" "$work/"
else
	base="https://github.com/$REPO/releases/download/$version"
	fetch "$base/SHA256SUMS" "$work/SHA256SUMS" ||
		fail "could not download $base/SHA256SUMS; while the repository is private, use --from with --version"
fi
grep -x "[0-9a-f]\{64\}  $(printf '%s' "$name" | sed 's/\./\\./g')" "$work/SHA256SUMS" >"$work/sum" ||
	fail "SHA256SUMS names no $name, the program of $version for linux $arch"
if [ -n "$from" ]; then
	[ -f "$from/$name" ] || fail "$from has no $name"
	cp "$from/$name" "$work/"
else
	fetch "$base/$name" "$work/$name" || fail "could not download $base/$name"
fi
(cd "$work" && sha256sum -c --status sum) ||
	fail "$name does not match its checksum in SHA256SUMS, so it is not installed"

# The checksums come from the same release as the program, so they catch a
# broken download, not a replaced release. From 3.0 each release also holds
# GitHub's attestation, signed for Mila's release workflow and the release's
# tag, which nobody can make for a program built anywhere else. gh checks it
# from the file alone, signed in or not, against the release chosen above.
BUNDLE=mila.intoto.jsonl
if [ -z "$attested" ]; then
	say "$version is from before releases were attested: only $name's checksum is"
	say "checked, not where it was built."
elif command -v gh >/dev/null && gh attestation verify --help >/dev/null 2>&1; then
	if [ -n "$from" ]; then
		[ -f "$from/$BUNDLE" ] && cp "$from/$BUNDLE" "$work/"
	else
		fetch "$base/$BUNDLE" "$work/$BUNDLE" || rm -f "$work/$BUNDLE"
	fi
	[ -f "$work/$BUNDLE" ] ||
		fail "the release has no $BUNDLE, which says where $name was built, so it is not installed${from:+ (download it into $from too)}"
	if ! GH_NO_UPDATE_NOTIFIER=1 GH_PROMPT_DISABLED=1 gh attestation verify "$work/$name" \
		--bundle "$work/$BUNDLE" --repo "$REPO" \
		--signer-workflow "$REPO/.github/workflows/desktop.yml" \
		--source-ref "refs/tags/$version" --deny-self-hosted-runners >"$work/attestation.log" 2>&1; then
		sed 's/^/  /' "$work/attestation.log" >&2
		fail "$name was not shown to be built by Mila's release workflow from $version, so it is not installed"
	fi
	say "Checked: $name was built by Mila's release workflow from $version."
else
	say "Only $name's checksum is checked, which a replaced release would pass too:"
	say "without gh 2.49 or later, nothing here shows that Mila's release workflow"
	say "built it. Install gh and run this again, or check it by hand as the guide shows."
fi
tar -xzf "$work/$name" -C "$work"
dir=$work/${name%.tar.gz}
[ -x "$dir/mila" ] && [ -f "$dir/mila.service" ] || fail "$name does not hold the program and its unit"
new=$want

# On a first install, the questions, before anything is installed.
upgrade=''
[ -f "$UNIT" ] && upgrade=yes
if [ -z "$upgrade" ]; then
	while [ -z "$domain" ]; do
		[ -z "$yes" ] || fail "--yes needs --domain on a first install"
		has_tty || fail "there is no terminal to ask on: give the domain with --domain"
		domain=$(ask "The domain people will use for Mila, such as mila.example.com:" '')
	done
	domain=$(printf '%s' "$domain" | tr '[:upper:]' '[:lower:]' | sed 's|^https\{0,1\}://||; s|/.*$||')
	case $domain in
	*[!a-z0-9.-]* | .* | *. | *..* | '') fail "$domain is not a domain name, such as mila.example.com" ;;
	*.*) ;;
	*) fail "$domain is not a domain name, such as mila.example.com" ;;
	esac
	if [ -z "$proxy" ]; then
		busy=''
		if command -v ss >/dev/null; then
			ss -Hltn 'sport = :80 or sport = :443' 2>/dev/null | grep -q . && busy=yes
		fi
		if [ -n "$busy" ]; then
			say "Something already listens on port 80 or 443, such as a proxy."
			a=$(ask "Run Mila behind it, on 127.0.0.1:8080? [Y/n]" y)
			case $a in [nN]*) ;; *) proxy=yes ;; esac
		else
			a=$(ask "Mila can get its own HTTPS certificate, if $domain points here and ports 80 and 443 are open. Do that? [Y/n]" y)
			case $a in [nN]*) proxy=yes ;; *) ;; esac
		fi
	fi
	if [ -z "$proxy" ]; then
		[ -n "$email" ] || email=$(ask "An email address for Let's Encrypt's notices about the certificate (optional):" '')
		case $email in
		'' | [!@]*@*.*) ;;
		*) fail "$email is not an email address" ;;
		esac
		case $email in *[!A-Za-z0-9.@+_-]*) fail "$email is not an email address" ;; esac
	fi
	# A settings file kept from an earlier install (--uninstall leaves it) is
	# used as it is. Mila's own HTTPS cannot start with an address other than
	# https://<the domain>, and nothing stands in front of it to trust; behind a
	# proxy, another address may be meant (the home network's), so it is said.
	if [ -f "$CONF" ]; then
		kept_url=$(sed -n 's/^[[:space:]]*base_url[[:space:]]*=[[:space:]]*//p' "$CONF" | tail -n 1 |
			sed 's/[[:space:]]*#.*$//; s/^["'"'"']//; s/["'"'"'][[:space:]]*$//')
		# As Mila reads it: the scheme and host count, a path and :443 do not.
		kept_site=$(printf '%s' "$kept_url" | tr '[:upper:]' '[:lower:]' | sed 's|^\([a-z]*://[^/]*\).*$|\1|; s|:443$||')
		kept_proxies=''
		grep -q '^[[:space:]]*trusted_proxies[[:space:]]*=' "$CONF" && kept_proxies=yes
		if [ -z "$proxy" ]; then
			wrong=''
			if [ -n "$kept_url" ] && [ "$kept_site" != "https://$domain" ]; then
				wrong="base_url is $kept_url, where Mila's own HTTPS needs https://$domain or none"
			fi
			if [ -n "$kept_proxies" ]; then
				wrong="${wrong:+$wrong, and }trusted_proxies trusts a proxy, where nothing stands in front of Mila"
			fi
			[ -z "$wrong" ] || fail "$CONF, kept from an earlier install, is used as it is: $wrong. Change or take out those lines, then run this again"
		elif [ -n "$kept_url" ] && [ "$kept_site" != "https://$domain" ]; then
			say "$CONF, kept from an earlier install, is used as it is: Mila's address stays"
			say "$kept_url, not https://$domain. Change base_url there if that is wrong."
		fi
	fi
fi

# The settings file, with lines given; one already there is left as it is.
write_conf() { # lines
	[ -f "$CONF" ] && return 0
	mkdir -p /etc/mila
	# Readable by root alone from the start, since it may hold passwords
	# carried from the old unit, and only then given to the mila group.
	rm -f "$CONF.new"
	(
		umask 077
		{
			say "# Mila's settings. docs/self-hosting.md lists every one; each is the name of"
			say "# its MILA_ variable in lower case, without MILA_ (MILA_SMTP_URL is smtp_url)."
			say "# Passwords belong here, readable by root and Mila alone, rather than in a"
			say "# drop-in's Environment= lines, which systemd shows to anyone on the computer."
			say "# After a change: sudo systemctl restart mila"
			say ""
			printf '%s' "$1"
		} >"$CONF.new"
	)
	chown root:mila "$CONF.new"
	chmod 640 "$CONF.new"
	mv -f "$CONF.new" "$CONF"
}

# Points the unit at the settings file. The unit's own example address is
# taken out, since a variable would win over the file.
write_dropin() {
	{
		say "# Mila's settings are in $CONF."
		say "[Service]"
		say "Environment=MILA_CONFIG=$CONF"
		say "Environment=MILA_BASE_URL="
	} >"$DROPINS/settings.conf"
	chmod 644 "$DROPINS/settings.conf"
}

# A first install's answers.
write_settings() {
	if [ -z "$proxy" ]; then
		sed -e "s|^Environment=MILA_TLS_DOMAIN=.*|Environment=MILA_TLS_DOMAIN=$domain|" \
			-e "s|^Environment=MILA_TLS_EMAIL=.*|Environment=MILA_TLS_EMAIL=$email|" \
			"$dir/mila-https.conf" >"$DROPINS/https.conf"
		chmod 644 "$DROPINS/https.conf"
		write_conf ''
	else
		# Behind a proxy on this computer, Mila trusts it alone to say who is
		# asking, so each visitor has a sign-in limit of their own.
		write_conf "base_url = \"https://$domain\"
trusted_proxies = \"127.0.0.1/32,::1/128\"
"
	fi
	write_dropin
}

# A TOML string of a value.
toml_string() { printf '"%s"' "$(printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')"; }

# Installing.

if ! id mila >/dev/null 2>&1; then
	nologin=$(command -v nologin || echo /bin/false)
	useradd --system --home-dir "$DATA" --no-create-home --shell "$nologin" mila
elif ! system_mila; then
	fail "a user called mila already exists and is not a system account; Mila would own its files. Rename it, or remove it, first"
fi

install -m 755 "$dir/mila" "$BIN.new"
mv -f "$BIN.new" "$BIN"

mkdir -p "$DROPINS"
if [ -n "$upgrade" ]; then
	# Settings written into the unit itself, as the guide once said, move to a
	# drop-in before the unit is replaced, so the upgrade keeps them.
	if ! [ -f "$DROPINS/settings.conf" ] && ! [ -f "$DROPINS/https.conf" ] &&
		! cmp -s "$UNIT" "$dir/mila.service"; then
		kept=$(grep '^Environment=' "$UNIT" || true)
		shipped=$(grep '^Environment=' "$dir/mila.service" || true)
		if [ -n "$kept" ] && [ "$kept" != "$shipped" ]; then
			# The unit's own three stay variables, since the unit sets them too;
			# the rest, passwords perhaps among them, go in the settings file.
			conf='' env=''
			while IFS= read -r line; do
				kv=${line#Environment=}
				kv=${kv#\"}
				kv=${kv%\"}
				key=${kv%%=*}
				value=${kv#*=}
				case $key in
				MILA_DATA_DIR | MILA_LISTEN | MILA_BASE_URL) env="${env}Environment=$key=$value
" ;;
				MILA_*) conf="${conf}$(printf '%s' "${key#MILA_}" | tr '[:upper:]' '[:lower:]') = $(toml_string "$value")
" ;;
				esac
			done <<EOF_KEPT
$kept
EOF_KEPT
			write_conf "$conf"
			{
				say "# Settings carried from the unit as it was before an upgrade; the"
				say "# rest of them are in $CONF."
				say "[Service]"
				say "Environment=MILA_CONFIG=$CONF"
				printf '%s' "$env"
			} >"$DROPINS/settings.conf"
			chmod 644 "$DROPINS/settings.conf"
			say "The settings in the old unit are kept: passwords and the like in $CONF, the rest in $DROPINS/settings.conf."
		fi
	fi
	# A password written into a drop-in, as the guide once said, is shown by
	# systemd to anyone on the computer.
	if grep -qs '^Environment=.*://[^/ ]*:[^@/ ]*@' "$DROPINS"/*.conf; then
		say "A drop-in in $DROPINS holds a password in a URL, which systemd shows to anyone"
		say "on this computer. Move that setting into $CONF (its name in lower case, without"
		say "MILA_), point the unit at it with Environment=MILA_CONFIG=$CONF, and restart Mila."
	fi
	install -m 644 "$dir/mila.service" "$UNIT"
else
	install -m 644 "$dir/mila.service" "$UNIT"
	write_settings
fi

systemctl daemon-reload
since=$(date '+%Y-%m-%d %H:%M:%S')
systemctl enable mila >/dev/null 2>&1
systemctl restart mila

# Waiting for it, and the first account's link.
link=''
i=0
while [ $i -lt 30 ]; do
	sleep 1
	i=$((i + 1))
	log=$(journalctl -u mila --since "$since" -o cat 2>/dev/null || true)
	link=$(printf '%s\n' "$log" | grep -o 'https\{0,1\}://[^" ]*/setup?token=[A-Za-z0-9_-]*' | tail -n 1 || true)
	if printf '%s\n' "$log" | grep -q '"msg":"listening"\|msg=listening'; then
		[ -n "$link" ] || sleep 1
		link=$(journalctl -u mila --since "$since" -o cat 2>/dev/null | grep -o 'https\{0,1\}://[^" ]*/setup?token=[A-Za-z0-9_-]*' | tail -n 1 || true)
		break
	fi
	systemctl is-active --quiet mila || break
done
if ! systemctl is-active --quiet mila; then
	say "Mila did not start. What it said:"
	journalctl -u mila --since "$since" -n 20 -o cat >&2 || true
	exit 1
fi

if [ -n "$upgrade" ]; then
	say "Mila is upgraded${new:+ to $new}, and running."
else
	say "Mila${new:+ $new} is installed and running."
fi
if [ -n "$link" ]; then
	say ""
	say "Open this link to make the first account. It works once:"
	say "  $link"
fi
if [ -n "$proxy" ] && [ -z "$upgrade" ]; then
	say ""
	say "Point your proxy for $domain at http://127.0.0.1:8080, with WebSockets on."
	say "docs/self-hosting.md, option 2, has the rest."
elif [ -z "$upgrade" ]; then
	say ""
	say "Mila is getting its certificate for $domain. If the page does not open over"
	say "HTTPS in a minute: journalctl -u mila says why."
fi
say ""
say "Settings: $CONF   Data: $DATA   Log: journalctl -u mila"
