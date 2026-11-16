# Testing Mila

This is a staged test build of Mila. Pull and run the newest one with the staging tool:

```bash
python <Staging>/_tools/stage.py pull mila --run      # Linux
```

```text
python <Staging>\_tools\stage.py pull mila --run      # Windows
```

On a system where `python` is not Python 3 (Fedora, Ubuntu), pull without `--run` and start it yourself: `python3 ~/.local/stage/mila/stage/run.py`.

The browser opens on its own. The first time, it opens the setup page; after that, Mila itself. Press Ctrl+C in the terminal to stop.

## Things to try

- **Sign in on a second device.** Open Mila in a private window or another browser, ask for an email link, and copy the link boxed in the terminal into that window. Then change things in one window and watch the other.
- **Go offline.** In the browser's developer tools, set the network to Offline, keep working, then go back online. The dot at the bottom of the sidebar says what is happening.
- **Passkeys.** Settings, Security, Add a passkey; then sign out and sign in with it.
- **Invite someone.** Create a shared workspace from the workspace menu, invite an address from Settings, Workspace, and open the boxed invitation link in a private window.

## Options

```text
python stage/run.py --port 9000   another port (it is remembered for next time)
python stage/run.py --fresh       delete the test data and start over
python stage/run.py --lan         reachable from your phone on the same network
```

Without `--port`, Mila uses the port it used last time, or else 8790 or the next free port after it. Windows sometimes sets aside whole blocks of ports (for Hyper-V, WSL or Docker) and refuses them with a permissions error; `netsh interface ipv4 show excludedportrange protocol=tcp` lists them.

With `--lan`, Mila is plain HTTP on an IP address, so phones will not offer passkeys there and the app will not open offline on the phone. Everything else works.

Test data is kept between builds: `~/.local/share/mila-test` on Linux, `%LOCALAPPDATA%\mila-test` on Windows.
