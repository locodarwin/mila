//! A place on this computer: the same Mila program the servers run, here with SQLite,
//! on this computer's own address, from when it is first opened until the app quits
//! (docs/PLAN.md section 3). Each place has a folder and a port of its own.

use crate::signin::{challenge_of, random_token};
use std::net::TcpListener;
use std::path::Path;
use std::process::{Child, Command, Stdio};
use std::time::{Duration, Instant};

const START_TIMEOUT: Duration = Duration::from_secs(60);

pub struct Server {
    child: Child,
    pub origin: String,
    pub key: String,
    /// What the app asks for its news with while the window shows another place.
    pub watch: String,
}

impl Drop for Server {
    fn drop(&mut self) {
        // It stops when its input closes, and so also if this program dies some other
        // way. Give it a moment to finish, since it may be writing.
        drop(self.child.stdin.take());
        let deadline = Instant::now() + Duration::from_secs(5);
        while Instant::now() < deadline {
            if matches!(self.child.try_wait(), Ok(Some(_))) {
                return;
            }
            std::thread::sleep(Duration::from_millis(50));
        }
        let _ = self.child.kill();
    }
}

/// A free port of this computer's, or the given one if nothing else has it.
pub fn free_port(wanted: Option<u16>) -> std::io::Result<u16> {
    if let Some(port) = wanted {
        if TcpListener::bind(("127.0.0.1", port)).is_ok() {
            return Ok(port);
        }
    }
    Ok(TcpListener::bind(("127.0.0.1", 0))?.local_addr()?.port())
}

/// The Mila program, which the app's installer puts beside the app itself.
fn program() -> std::io::Result<std::path::PathBuf> {
    let me = std::env::current_exe()?;
    let dir = me.parent().ok_or(std::io::ErrorKind::NotFound)?;
    Ok(dir.join(if cfg!(windows) { "mila.exe" } else { "mila" }))
}

/// Whether a variable of the environment is one of Mila's settings. Windows reads names
/// in any case, so `Mila_OAuth` is `MILA_OAUTH` there, and is left out as well.
fn is_milas(name: &std::ffi::OsStr) -> bool {
    name.to_string_lossy().get(..5).is_some_and(|prefix| prefix.eq_ignore_ascii_case("MILA_"))
}

/// Starts the server for the place whose folder this is, and waits until it answers.
pub fn start(home: &Path, port: u16) -> Result<Server, String> {
    let data = home.join("data");
    std::fs::create_dir_all(&data).map_err(|e| format!("Could not make {}: {e}", data.display()))?;
    let log = std::fs::File::create(home.join("server.log")).map_err(|e| format!("Could not write the log: {e}"))?;
    let log2 = log.try_clone().map_err(|e| e.to_string())?;
    let origin = format!("http://127.0.0.1:{port}");
    let key = random_token(32);
    let watch = random_token(32);
    let mut command = Command::new(program().map_err(|e| e.to_string())?);
    // Nothing the shell around it set may leak in and change what it is.
    for (name, _) in std::env::vars_os() {
        if is_milas(&name) {
            command.env_remove(name);
        }
    }
    command
        .arg("serve")
        .env("MILA_LOCAL", "on")
        // In its environment: a flag would show in the list of processes.
        .env("MILA_LOCAL_KEY", &key)
        .env("MILA_LOCAL_WATCH_KEY", &watch)
        .env("MILA_EXIT_WITH_STDIN", "on")
        .env("MILA_LISTEN", format!("127.0.0.1:{port}"))
        .env("MILA_BASE_URL", &origin)
        .env("MILA_DATA_DIR", &data)
        .env("MILA_LOG_FORMAT", "text")
        .stdin(Stdio::piped())
        .stdout(log)
        .stderr(log2);
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        command.creation_flags(0x0800_0000); // CREATE_NO_WINDOW
    }
    let mut child = command.spawn().map_err(|e| format!("Could not start Mila: {e}"))?;
    let deadline = Instant::now() + START_TIMEOUT;
    let health = format!("{origin}/healthz");
    loop {
        if let Ok(Some(status)) = child.try_wait() {
            return Err(format!("Mila stopped as it started ({status}). See server.log in {}.", home.display()));
        }
        let agent = ureq::AgentBuilder::new().timeout(Duration::from_secs(2)).build();
        if let Ok(answer) = agent.get(&health).call() {
            // It has to be the Mila just started, which knows the key, before the key
            // goes anywhere: something else on this computer could have taken the port.
            if answer.header("X-Mila-Local-Proof") != Some(challenge_of(&key).as_str()) {
                let _ = child.kill();
                return Err(format!("Something else on this computer is using port {port}."));
            }
            return Ok(Server { child, origin, key, watch });
        }
        if Instant::now() > deadline {
            let _ = child.kill();
            return Err(format!("Mila took too long to start. See server.log in {}.", home.display()));
        }
        std::thread::sleep(Duration::from_millis(100));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_port_stays_the_same_while_its_free() {
        let port = free_port(None).unwrap();
        assert_eq!(free_port(Some(port)).unwrap(), port);
    }

    #[test]
    fn milas_settings_are_left_out_in_any_case() {
        for name in ["MILA_OAUTH", "Mila_OAuth", "mila_smtp_url", "MILA_"] {
            assert!(is_milas(std::ffi::OsStr::new(name)), "{name}");
        }
        for name in ["PATH", "MILAX", "MIL", "HOME", "XMILA_OAUTH", "ÉMILA_X"] {
            assert!(!is_milas(std::ffi::OsStr::new(name)), "{name}");
        }
    }

    #[test]
    fn a_taken_port_gives_way_to_another() {
        let held = TcpListener::bind(("127.0.0.1", 0)).unwrap();
        let port = held.local_addr().unwrap().port();
        assert_ne!(free_port(Some(port)).unwrap(), port);
    }
}
