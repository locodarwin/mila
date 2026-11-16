#!/usr/bin/env python3
"""Runs a staged build of Mila on this machine, for testing.

    python stage/run.py              start Mila and open it in the browser
    python stage/run.py --port 9000  use another port (default: the last one
                                     used, else 8790 or the next free one)
    python stage/run.py --lan        also reachable from other devices on the network
    python stage/run.py --fresh      start over with no accounts or data

Needs only Python 3. Data lives outside the install, so pulling a new build
keeps your test accounts and tasks: ~/.local/share/mila-test on Linux,
%LOCALAPPDATA%\\mila-test on Windows. There is no email: sign-in links and
invitations are printed here instead, boxed so they are easy to spot.
"""

from __future__ import annotations

import argparse
import json
import os
import platform
import re
import shutil
import socket
import stat
import subprocess
import sys
import webbrowser
from pathlib import Path

HERE = Path(__file__).resolve().parent
INSTALL = HERE.parent
URL = re.compile(r"https?://[^\s\"\\]+")


def data_dir() -> Path:
    if os.name == "nt":
        return Path(os.environ.get("LOCALAPPDATA", str(Path.home()))) / "mila-test"
    return Path(os.environ.get("XDG_DATA_HOME", str(Path.home() / ".local" / "share"))) / "mila-test"


def binary() -> Path:
    if sys.platform == "darwin":
        sys.exit("This test build has no macOS binary.")
    system = "windows" if os.name == "nt" else "linux"
    arch = "arm64" if platform.machine().lower() in ("aarch64", "arm64") else "amd64"
    path = HERE / "bin" / f"mila-{system}-{arch}{'.exe' if system == 'windows' else ''}"
    if not path.is_file():
        sys.exit(f"No Mila binary for {system}/{arch} in this build ({path}).")
    if os.name != "nt" and not os.access(path, os.X_OK):
        path.chmod(path.stat().st_mode | stat.S_IXUSR)
    return path


def lan_address() -> str:
    """This machine's address on the local network."""
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(("192.0.2.1", 9))  # no packet is sent; this only picks a route
        return s.getsockname()[0]
    except OSError:
        return "127.0.0.1"
    finally:
        s.close()


def bind_error(host: str, port: int) -> OSError | None:
    """Why host:port cannot be listened on, or None if it can."""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.bind((host, port))
        return None
    except OSError as e:
        return e
    finally:
        s.close()


def why(err: OSError) -> str:
    # Windows refuses ports it has set aside (Hyper-V, WSL, Docker) as a
    # permissions error rather than as taken.
    if os.name == "nt" and getattr(err, "winerror", None) == 10013:
        return ("Windows has reserved it; netsh interface ipv4 show excludedportrange "
                "protocol=tcp lists the reserved ranges")
    return str(err.strerror or err)


def free_port(host: str, wanted: int | None, remembered: Path) -> int:
    """The port to use: the one asked for, or else the one used last time,
    or else 8790 or the first free one after it. Windows can set aside
    blocks of thousands of ports, so the search goes a long way up."""
    if wanted is not None:
        err = bind_error(host, wanted)
        if err:
            sys.exit(f"Mila cannot listen on port {wanted}: {why(err)}.")
        return wanted
    try:
        last = int(remembered.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        last = None
    # The same port each time keeps the same address, and the browser
    # keeps a site's offline copy and sign-in per address.
    if last is not None and bind_error(host, last) is None:
        return last
    first = 8790
    first_err = bind_error(host, first)
    if first_err is None:
        return first
    for port in range(first + 1, first + 5000):
        if bind_error(host, port) is None:
            print(f"Port {first} cannot be used ({why(first_err)}), so Mila uses {port}.")
            return port
    sys.exit(f"No free port from {first} to {first + 4999}. Try: python stage/run.py --port 20000")


def box(title: str, lines: list[str]) -> None:
    width = max(len(title), *(len(l) for l in lines)) + 4
    print("\n+" + "-" * width + "+")
    print(f"|  {title.ljust(width - 2)}|")
    for line in lines:
        print(f"|  {line.ljust(width - 2)}|")
    print("+" + "-" * width + "+\n", flush=True)


def field(line: str, name: str) -> str:
    m = re.search(name + r'=("(?:[^"\\]|\\.)*"|\S+)', line)
    return m.group(1).strip('"') if m else ""


def main() -> int:
    parser = argparse.ArgumentParser(description="Run a staged build of Mila for testing.")
    parser.add_argument("--port", type=int, help="port to listen on (default 8790, or the next free one)")
    parser.add_argument("--lan", action="store_true", help="listen on the network too (no passkeys; see the README)")
    parser.add_argument("--fresh", action="store_true", help="delete the test data and start over")
    parser.add_argument("--no-browser", action="store_true", help="don't open the browser")
    args = parser.parse_args()

    data = data_dir()
    if args.fresh and data.exists():
        shutil.rmtree(data)
        print(f"Deleted the test data in {data}.")

    host = "0.0.0.0" if args.lan else "127.0.0.1"
    remembered = data / "run-port"
    port = free_port(host, args.port, remembered)
    try:
        data.mkdir(parents=True, exist_ok=True)
        remembered.write_text(str(port), encoding="utf-8")
    except OSError:
        pass
    listen = f"{host}:{port}"
    base = f"http://{lan_address()}:{port}" if args.lan else f"http://localhost:{port}"

    build = "unknown build"
    record = INSTALL / ".stage-build.json"
    if record.is_file():
        r = json.loads(record.read_text(encoding="utf-8"))
        build = f"build {r.get('build_id', '?')} (commit {str(r.get('commit', '?'))[:8]}, {r.get('built_at', '?')})"
    box("Mila test build", [build, f"Data: {data}", f"Address: {base}", "Stop with Ctrl+C."])

    cmd = [str(binary()), "serve", "--listen", listen, "--base-url", base,
           "--data-dir", str(data), "--log-format", "text"]
    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1)
    opened = False
    setup_seen = False
    try:
        assert proc.stdout is not None
        for line in proc.stdout:
            if "Open this one-time link" in line:
                setup_seen = True
                url = URL.search(line)
                if url:
                    box("First run: create your account", [url.group(0)])
                    if not args.no_browser:
                        webbrowser.open(url.group(0))
                        opened = True
            elif "message written to the log" in line:
                url = URL.search(line)
                box(f"Email to {field(line, 'to')}: {field(line, 'subject')}",
                    [url.group(0) if url else "(no link)", "Open this link in the browser that should sign in."])
            elif "msg=listening" in line:
                print(f"Mila is running at {base}", flush=True)
                if not opened and not setup_seen and not args.no_browser:
                    webbrowser.open(base)
                    opened = True
            elif "listen tcp" in line:
                # Taken between the check above and Mila starting.
                print(line, end="")
                print(f"\nPort {port} cannot be used. Try: python stage/run.py --port {port + 1}", flush=True)
            else:
                print(line, end="", flush=True)
        return proc.wait()
    except KeyboardInterrupt:
        proc.terminate()
        proc.wait()
        print("\nMila stopped. Your test data is kept for next time.")
        return 0


if __name__ == "__main__":
    sys.exit(main())
