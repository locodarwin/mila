//! Signing in through the person's own browser (docs/PLAN.md section 3): the Linux
//! webview has no WebAuthn, so the app opens the browser at the server, and the
//! browser hands a one-time code to a port on this computer only. The code is no use
//! without the verifier, a secret that never leaves this program (RFC 7636 and 8252).

use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use sha2::{Digest, Sha256};
use std::io::{BufRead, BufReader, Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};

/// How long to wait for the person to finish in their browser.
const WAIT: Duration = Duration::from_secs(600);
/// As much of a request line as is worth reading.
const REQUEST_LINE: u64 = 8192;

pub struct Pending {
    pub listener: TcpListener,
    pub port: u16,
    pub state: String,
    pub verifier: String,
    pub challenge: String,
}

/// A random string for use in an address.
pub fn random_token(bytes: usize) -> String {
    let mut raw = vec![0u8; bytes];
    getrandom::fill(&mut raw).expect("the system's random numbers");
    URL_SAFE_NO_PAD.encode(raw)
}

/// The challenge sent ahead for a verifier: its SHA-256, as RFC 7636's S256 has it.
pub fn challenge_of(verifier: &str) -> String {
    URL_SAFE_NO_PAD.encode(Sha256::digest(verifier.as_bytes()))
}

/// Starts listening on a port of this computer alone.
pub fn begin() -> std::io::Result<Pending> {
    let listener = TcpListener::bind(("127.0.0.1", 0))?;
    let port = listener.local_addr()?.port();
    let verifier = random_token(48);
    Ok(Pending {
        listener,
        port,
        state: random_token(24),
        challenge: challenge_of(&verifier),
        verifier,
    })
}

/// The address to open in the browser.
pub fn browser_url(server: &str, p: &Pending) -> String {
    format!(
        "{}/desktop?port={}&state={}&challenge={}",
        server.trim_end_matches('/'),
        p.port,
        p.state,
        p.challenge
    )
}

/// What a request to the port said.
#[derive(Debug, PartialEq)]
pub enum Heard {
    Code(String),
    /// The right sign-in, but finished at another server than the one asked for.
    OtherServer,
    Nothing,
}

/// Reads a code out of the request line the browser sends, if it's the one this sign-in
/// is waiting for. Anything else on this computer can reach the port, so the state has
/// to match, and only then does the page say anything useful. The server's page says
/// which server it is (RFC 9207): a server the person named could send their browser on
/// to another they use, and be handed that one's code.
pub fn code_from_request(line: &str, state: &str, server: &str) -> Heard {
    let Some(parsed) = line
        .strip_prefix("GET ")
        .and_then(|rest| rest.split(' ').next())
        .and_then(|target| url::Url::parse(&format!("http://127.0.0.1{target}")).ok())
    else {
        return Heard::Nothing;
    };
    let (mut code, mut iss, mut matched) = (None, None, false);
    for (k, v) in parsed.query_pairs() {
        match k.as_ref() {
            "code" => code = Some(v.into_owned()),
            "iss" => iss = url::Url::parse(&v).ok().map(|u| u.origin().ascii_serialization()),
            "state" => matched = v == state,
            _ => {}
        }
    }
    match code.filter(|c| matched && !c.is_empty()) {
        Some(_) if iss.as_deref() != Some(server) => Heard::OtherServer,
        Some(code) => Heard::Code(code),
        None => Heard::Nothing,
    }
}

const OTHER_SERVER: &str = "That sign-in came from a different server than the one you gave the app, so the app ignored it.";

const DONE: &str = "<!doctype html><meta charset=utf-8><title>Signed in</title><body style='font:16px system-ui;margin:3rem'><h1 style='font-size:1.3rem'>You're signed in</h1><p>You can close this tab and go back to the Mila app.</p>";

fn answer(mut stream: &TcpStream, status: &str, body: &str) {
    let _ = write!(
        stream,
        "HTTP/1.1 {status}\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: {}\r\nConnection: close\r\nCache-Control: no-store\r\nReferrer-Policy: no-referrer\r\n\r\n{body}",
        body.len()
    );
}

/// Waits for the browser to hand over the code from this server. None if canceled or
/// out of time. Each connection is read on a thread of its own, so another program on
/// this computer that connects and says nothing cannot hold up the browser's.
pub fn wait_for_code(p: &Pending, server: &str, cancel: &Arc<AtomicBool>) -> Result<Option<String>, String> {
    p.listener.set_nonblocking(true).map_err(|e| e.to_string())?;
    let deadline = Instant::now() + WAIT;
    let (heard, lines) = std::sync::mpsc::channel::<(TcpStream, String)>();
    let mut reading = 0usize;
    while Instant::now() < deadline && !cancel.load(Ordering::SeqCst) {
        while let Ok((stream, line)) = lines.try_recv() {
            reading -= 1;
            match code_from_request(line.trim_end(), &p.state, server) {
                Heard::Code(code) => {
                    answer(&stream, "200 OK", DONE);
                    return Ok(Some(code));
                }
                Heard::OtherServer => {
                    answer(&stream, "400 Bad Request", OTHER_SERVER);
                    return Err(OTHER_SERVER.to_string());
                }
                Heard::Nothing => answer(&stream, "404 Not Found", ""),
            }
        }
        match p.listener.accept() {
            // A few at once at most: each one waits its read timeout out.
            Ok((stream, _)) if reading < MAX_READING => {
                reading += 1;
                let heard = heard.clone();
                std::thread::spawn(move || {
                    let _ = stream.set_nonblocking(false);
                    let _ = stream.set_read_timeout(Some(Duration::from_secs(5)));
                    let mut line = String::new();
                    let ok = BufReader::new((&stream).take(REQUEST_LINE)).read_line(&mut line).is_ok();
                    let _ = heard.send((stream, if ok { line } else { String::new() }));
                });
            }
            Ok(_) => {}
            Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                std::thread::sleep(Duration::from_millis(50));
            }
            Err(e) => return Err(e.to_string()),
        }
    }
    Ok(None)
}

/// How many connections are read at once while waiting for the browser.
const MAX_READING: usize = 16;

#[cfg(test)]
mod tests {
    use super::*;

    const HOME: &str = "https://home.example";

    #[test]
    fn a_code_needs_its_state() {
        let hear = |line| code_from_request(line, "right", HOME);
        let line = "GET /?code=abc&state=right&iss=https%3A%2F%2Fhome.example HTTP/1.1";
        assert_eq!(hear(line), Heard::Code("abc".into()));
        assert_eq!(code_from_request(line, "wrong", HOME), Heard::Nothing);
        assert_eq!(hear("GET /?code=abc HTTP/1.1"), Heard::Nothing);
        assert_eq!(hear("GET /?state=right HTTP/1.1"), Heard::Nothing);
        assert_eq!(hear("GET /?code=&state=right HTTP/1.1"), Heard::Nothing);
        assert_eq!(hear("POST /?code=abc&state=right HTTP/1.1"), Heard::Nothing);
        assert_eq!(hear("nonsense"), Heard::Nothing);
    }

    #[test]
    fn a_code_from_another_server_is_refused() {
        let hear = |line| code_from_request(line, "right", HOME);
        assert_eq!(hear("GET /?code=abc&state=right&iss=https%3A%2F%2Fother.example HTTP/1.1"), Heard::OtherServer);
        assert_eq!(hear("GET /?code=abc&state=right HTTP/1.1"), Heard::OtherServer, "it must say");
        assert_eq!(hear("GET /?code=abc&state=right&iss=nonsense HTTP/1.1"), Heard::OtherServer);
        assert_eq!(hear("GET /?code=abc&state=wrong&iss=https%3A%2F%2Fother.example HTTP/1.1"), Heard::Nothing, "only this sign-in's");
    }

    #[test]
    fn a_code_is_read_as_it_was_written() {
        let line = "GET /?code=a%2Bb&state=s&iss=https://home.example HTTP/1.1";
        assert_eq!(code_from_request(line, "s", HOME), Heard::Code("a+b".into()));
    }

    #[test]
    fn the_challenge_is_s256() {
        // RFC 7636's own example.
        assert_eq!(
            challenge_of("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"),
            "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
        );
    }

    #[test]
    fn secrets_differ_and_read_well_in_an_address() {
        let (a, b) = (random_token(24), random_token(24));
        assert_ne!(a, b);
        assert!(a.chars().all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_'));
    }

    #[test]
    fn it_goes_to_the_servers_own_page() {
        let p = begin().unwrap();
        let url = browser_url("https://mila.example.com/", &p);
        assert!(url.starts_with("https://mila.example.com/desktop?port="));
        assert!(url.contains(&p.challenge) && url.contains(&p.state));
        assert!(!url.contains(&p.verifier), "the secret never goes to the browser");
    }

    #[test]
    fn the_port_is_this_computers_alone() {
        let p = begin().unwrap();
        assert!(p.listener.local_addr().unwrap().ip().is_loopback());
    }

    #[test]
    fn it_stops_waiting_when_canceled() {
        let p = begin().unwrap();
        let cancel = Arc::new(AtomicBool::new(true));
        assert_eq!(wait_for_code(&p, HOME, &cancel), Ok(None));
    }

    #[test]
    fn it_ignores_what_isnt_the_browser_then_takes_the_code() {
        let p = begin().unwrap();
        let port = p.port;
        let state = p.state.clone();
        let client = std::thread::spawn(move || {
            let mut stray = TcpStream::connect(("127.0.0.1", port)).unwrap();
            write!(stray, "GET /?code=stolen&state=guess HTTP/1.1\r\n\r\n").unwrap();
            let mut text = String::new();
            std::io::Read::read_to_string(&mut stray, &mut text).unwrap();
            assert!(text.starts_with("HTTP/1.1 404"));
            let mut real = TcpStream::connect(("127.0.0.1", port)).unwrap();
            write!(real, "GET /?code=real&state={state}&iss={HOME} HTTP/1.1\r\n\r\n").unwrap();
            let mut text = String::new();
            std::io::Read::read_to_string(&mut real, &mut text).unwrap();
            assert!(text.contains("signed in"));
        });
        let cancel = Arc::new(AtomicBool::new(false));
        assert_eq!(wait_for_code(&p, HOME, &cancel), Ok(Some("real".to_string())));
        client.join().unwrap();
    }
}
