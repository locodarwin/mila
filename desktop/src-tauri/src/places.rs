//! Where a person's work lives: any number of servers and of places on this computer,
//! kept in a small file in the app's own folder.

use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
pub struct Places {
    /// Every place, in the order the Places screen lists them.
    #[serde(default)]
    pub list: Vec<Place>,
    /// The place opened last, by its id.
    #[serde(default)]
    pub last: Option<String>,
    /// Whether the app opens the last place at once rather than the Places screen.
    #[serde(default)]
    pub open_last: bool,
    /// As an app from before kept them, read once and written again for it, so that
    /// going back to an older version still finds the servers and the first place here.
    #[serde(default)]
    pub servers: Vec<String>,
    #[serde(default)]
    pub local_port: Option<u16>,
}

/// One place: a server, named by its origin, or one on this computer, "local" for the
/// first (whose work is in the app's own folder, as it always was) and "local-…" for
/// each one after it, in a folder of its own under `places`.
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Place {
    pub id: String,
    /// The person's own name for it, in this app only.
    #[serde(default)]
    pub name: Option<String>,
    /// A place on this computer's port, kept, since a page's storage belongs to its
    /// address, port included.
    #[serde(default)]
    pub port: Option<u16>,
    /// When it was last opened, in seconds since 1970.
    #[serde(default)]
    pub opened: Option<u64>,
}

impl Place {
    pub fn server(origin: &str) -> Place {
        Place { id: origin.to_string(), name: None, port: None, opened: None }
    }

    pub fn is_local(&self) -> bool {
        is_local(&self.id)
    }

    /// What the place shows as: its own name, or its address, or for one on this
    /// computer "On this computer".
    pub fn title(&self) -> String {
        match &self.name {
            Some(name) if !name.trim().is_empty() => name.trim().to_string(),
            _ if self.is_local() => "On this computer".to_string(),
            _ => name_of(&self.id),
        }
    }
}

pub fn is_local(id: &str) -> bool {
    id == LOCAL || id.starts_with("local-")
}

/// Whether an id is one the app makes for a place on this computer (`new_local_id`):
/// it names a folder, so nothing in it may climb out of `places`.
pub fn valid_local_id(id: &str) -> bool {
    id == LOCAL
        || id
            .strip_prefix("local-")
            .is_some_and(|rest| !rest.is_empty() && rest.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-'))
}

/// A place's id as read from the file, if it is one the app could have written: a place
/// on this computer's names its folder, and a server's is the origin `normalize_server`
/// keeps, which the window may show and its pages' commands are allowed to.
fn checked_id(id: &str) -> Option<String> {
    if is_local(id) {
        valid_local_id(id).then(|| id.to_string())
    } else {
        normalize_server(id).ok()
    }
}

pub const LOCAL: &str = "local";

/// The folder for data on this computer, which outlives the app.
pub fn data_home() -> PathBuf {
    let base = dirs::data_local_dir().unwrap_or_else(|| PathBuf::from("."));
    if cfg!(windows) {
        base.join("Mila")
    } else {
        base.join("mila")
    }
}

/// Makes the app's folder, for this person alone: it holds their work.
pub fn prepare_home(home: &Path) -> std::io::Result<()> {
    std::fs::create_dir_all(home)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(home, std::fs::Permissions::from_mode(0o700))?;
    }
    Ok(())
}

fn file(home: &Path) -> PathBuf {
    home.join("places.json")
}

pub fn load(home: &Path) -> Places {
    let mut places: Places = std::fs::read(file(home))
        .ok()
        .and_then(|raw| serde_json::from_slice(&raw).ok())
        .unwrap_or_default();
    // A file from before the Places screen: its servers, then the place on this computer.
    if places.list.is_empty() {
        places.list = places.servers.iter().map(|s| Place::server(s)).collect();
        if let Some(port) = places.local_port {
            places.list.push(Place { id: LOCAL.into(), name: None, port: Some(port), opened: None });
        }
    }
    checked(places)
}

/// What the file holds, without what the app would never have written there: it is the
/// person's own, but anything else wrong with this computer could change it.
fn checked(mut places: Places) -> Places {
    let mut seen = std::collections::HashSet::new();
    places.list = std::mem::take(&mut places.list)
        .into_iter()
        .filter_map(|mut place| {
            let Some(id) = checked_id(&place.id).filter(|id| seen.insert(id.clone())) else {
                eprintln!("places.json: left out a place that isn't one: {:?}", place.id);
                return None;
            };
            place.id = id;
            place.name = place.name.as_deref().and_then(tidy_name);
            place.port = place.port.filter(|p| *p != 0);
            Some(place)
        })
        .collect();
    places.last = places.last.as_deref().and_then(checked_id).filter(|last| places.list.iter().any(|p| &p.id == last));
    places
}

/// Written whole under another name first, so a crash never leaves half a file.
pub fn save(home: &Path, places: &Places) -> std::io::Result<()> {
    std::fs::create_dir_all(home)?;
    let mut written = places.clone();
    written.servers = places.list.iter().filter(|p| !p.is_local()).map(|p| p.id.clone()).collect();
    written.local_port = places.list.iter().find(|p| p.id == LOCAL).and_then(|p| p.port);
    let next = home.join("places.json.tmp");
    std::fs::write(&next, serde_json::to_vec_pretty(&written)?)?;
    std::fs::rename(next, file(home))
}

/// The folder a place on this computer keeps its work in: the app's own for the first,
/// as it always was, and one of its own under `places` for each after it.
pub fn local_dir(home: &Path, id: &str) -> PathBuf {
    if id == LOCAL {
        home.to_path_buf()
    } else {
        home.join("places").join(id)
    }
}

/// Deletes a place on this computer's work: its data and log, and its folder when it
/// has one of its own. The first place's folder is the app's, which holds the list of
/// places, so only its own parts go. Nothing outside the app's folder is touched.
pub fn delete_local(home: &Path, id: &str) -> std::io::Result<()> {
    if !is_local(id) || id.contains(['/', '\\', '.']) {
        return Err(std::io::Error::new(std::io::ErrorKind::InvalidInput, "not a place on this computer"));
    }
    let dir = local_dir(home, id);
    if id == LOCAL {
        for part in ["data", "server.log"] {
            let path = dir.join(part);
            if path.is_dir() {
                std::fs::remove_dir_all(&path)?;
            } else if path.exists() {
                std::fs::remove_file(&path)?;
            }
        }
        Ok(())
    } else if dir.exists() {
        std::fs::remove_dir_all(dir)
    } else {
        Ok(())
    }
}

/// A new place on this computer's id, one no place has.
pub fn new_local_id(places: &Places) -> String {
    if !places.list.iter().any(|p| p.id == LOCAL) {
        return LOCAL.to_string();
    }
    loop {
        let id = format!("local-{}", crate::signin::random_token(6).to_lowercase().replace(['-', '_'], "x"));
        if !places.list.iter().any(|p| p.id == id) {
            return id;
        }
    }
}

/// A name for a place the person gave, tidied: one line, not too long.
pub fn tidy_name(raw: &str) -> Option<String> {
    let name: String = raw.chars().filter(|c| !c.is_control()).take(60).collect();
    let name = name.trim();
    (!name.is_empty()).then(|| name.to_string())
}

/// A server's address as it is kept: its origin alone. Plain http is only for this
/// computer or a home network, where it's the person's own call; a sign-in's secrets
/// must not cross the internet in the clear, so a name with dots needs https.
pub fn normalize_server(raw: &str) -> Result<String, String> {
    let raw = raw.trim();
    if raw.is_empty() {
        return Err("Enter the address of your Mila server.".into());
    }
    let with_scheme = if raw.contains("://") {
        raw.to_string()
    } else {
        format!("https://{raw}")
    };
    let parsed = url::Url::parse(&with_scheme).map_err(|_| "That isn't an address.".to_string())?;
    let host = parsed.host_str().ok_or("That address has no host.")?;
    match parsed.scheme() {
        "https" => {}
        "http" if is_private(host) => {}
        "http" => return Err("A server on the internet needs an https address.".into()),
        _ => return Err("That isn't a web address.".into()),
    }
    if !parsed.username().is_empty() || parsed.password().is_some() {
        return Err("Leave the user name and password out of the address.".into());
    }
    Ok(parsed.origin().ascii_serialization())
}

fn is_private(host: &str) -> bool {
    if host == "localhost" || !host.contains('.') {
        return true;
    }
    match host.trim_matches(|c| c == '[' || c == ']').parse::<std::net::IpAddr>() {
        Ok(std::net::IpAddr::V4(ip)) => ip.is_loopback() || ip.is_private() || ip.is_link_local(),
        Ok(std::net::IpAddr::V6(ip)) => ip.is_loopback(),
        Err(_) => host.ends_with(".local") || host.ends_with(".lan") || host.ends_with(".internal"),
    }
}

/// The name a place shows under in the menu.
pub fn name_of(server: &str) -> String {
    server
        .trim_start_matches("https://")
        .trim_start_matches("http://")
        .to_string()
}

/// Whether a page's address belongs to a place: anywhere else opens in the browser.
pub fn same_origin(address: &url::Url, origin: &str) -> bool {
    address.origin().ascii_serialization() == origin
}

/// Whether a server's version is newer than this app's, comparing numbers and ignoring
/// what comes after them ("2.1.0-3-g2bb9988"). Anything not a version is not newer,
/// and a build with no version of its own (0.0.0, from a developer) is never told.
pub fn is_newer(server: &str, app: &str) -> bool {
    fn parts(v: &str) -> Option<[u64; 3]> {
        let v = v.trim_start_matches('v');
        let core = v.split(|c| c == '-' || c == '+').next()?;
        let mut it = core.split('.').map(|p| p.parse::<u64>().ok());
        Some([it.next()??, it.next()??, it.next()??])
    }
    match (parts(server), parts(app)) {
        (Some(s), Some(a)) => a != [0, 0, 0] && s > a,
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn addresses_are_tidied_to_an_origin() {
        assert_eq!(normalize_server("mila.example.com").unwrap(), "https://mila.example.com");
        assert_eq!(normalize_server(" https://mila.example.com/some/page?x=1 ").unwrap(), "https://mila.example.com");
        assert_eq!(normalize_server("https://mila.example.com:8443").unwrap(), "https://mila.example.com:8443");
    }

    #[test]
    fn plain_http_is_only_for_home() {
        for ok in ["http://localhost:8790", "http://127.0.0.1:8790", "http://192.168.1.20", "http://nas", "http://nas.local", "http://[::1]:8790"] {
            assert!(normalize_server(ok).is_ok(), "{ok}");
        }
        for bad in ["http://mila.example.com", "http://8.8.8.8", "ftp://mila.example.com", "file:///etc/passwd", "javascript:alert(1)", "", "https://user:pass@mila.example.com"] {
            assert!(normalize_server(bad).is_err(), "{bad}");
        }
    }

    #[cfg(unix)]
    #[test]
    fn the_folder_is_for_this_person_alone() {
        use std::os::unix::fs::PermissionsExt;
        let dir = std::env::temp_dir().join(format!("mila-home-{}", crate::signin::random_token(6)));
        std::fs::create_dir(&dir).unwrap();
        std::fs::set_permissions(&dir, std::fs::Permissions::from_mode(0o755)).unwrap();
        prepare_home(&dir).unwrap();
        assert_eq!(std::fs::metadata(&dir).unwrap().permissions().mode() & 0o777, 0o700);
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn places_survive_a_save() {
        let home = std::env::temp_dir().join(format!("mila-places-{}", std::process::id()));
        let places = Places {
            list: vec![Place::server("https://a.example"), Place { id: LOCAL.into(), name: Some("Mine".into()), port: Some(9000), opened: Some(5) }],
            last: Some(LOCAL.into()),
            open_last: true,
            ..Default::default()
        };
        save(&home, &places).unwrap();
        let back = load(&home);
        assert_eq!(back.list, places.list);
        assert_eq!((back.last, back.open_last), (places.last, true));
        // Written for an app from before, too.
        assert_eq!((back.servers, back.local_port), (vec!["https://a.example".to_string()], Some(9000)));
        std::fs::write(file(&home), b"not json").unwrap();
        assert_eq!(load(&home), Places::default(), "a broken file starts afresh");
        std::fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn a_file_from_before_keeps_its_places() {
        let home = std::env::temp_dir().join(format!("mila-old-{}", crate::signin::random_token(6)));
        std::fs::create_dir_all(&home).unwrap();
        std::fs::write(file(&home), br#"{"servers":["https://a.example","https://b.example"],"local_port":9000,"last":"https://b.example"}"#).unwrap();
        let places = load(&home);
        assert_eq!(places.list.iter().map(|p| p.id.as_str()).collect::<Vec<_>>(), ["https://a.example", "https://b.example", LOCAL]);
        assert_eq!(places.list[2].port, Some(9000));
        assert_eq!(places.last.as_deref(), Some("https://b.example"));
        assert!(!places.open_last, "the Places screen shows at the next start");
        std::fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn a_file_holds_only_places_the_app_could_have_written() {
        let home = std::env::temp_dir().join(format!("mila-bad-{}", crate::signin::random_token(6)));
        std::fs::create_dir_all(&home).unwrap();
        std::fs::write(
            file(&home),
            br#"{"list":[
                {"id":"local-../../.."},
                {"id":"local-a/b"},
                {"id":"local-"},
                {"id":"local-ab12","name":"Side\nproject","port":0},
                {"id":"javascript:alert(1)"},
                {"id":"http://mila.example.com"},
                {"id":"https://user:pass@mila.example.com"},
                {"id":"https://mila.example.com/some/page"},
                {"id":"https://mila.example.com"},
                {"id":"local"}
            ],"last":"local-../../.."}"#,
        )
        .unwrap();
        let places = load(&home);
        assert_eq!(places.list.iter().map(|p| p.id.as_str()).collect::<Vec<_>>(), ["local-ab12", "https://mila.example.com", LOCAL]);
        assert_eq!(places.list[0].name.as_deref(), Some("Sideproject"));
        assert_eq!(places.list[0].port, None, "no place is at port 0");
        assert_eq!(places.last, None, "only a place on the list is the last");
        std::fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn only_the_apps_own_ids_name_a_folder() {
        for ok in [LOCAL, "local-ab12", "local-x0x9"] {
            assert!(valid_local_id(ok), "{ok}");
        }
        for bad in ["local-", "local-../..", "local-a/b", "local-a\\b", "local-A", "local-a.b", "https://a.example", "locally"] {
            assert!(!valid_local_id(bad), "{bad}");
        }
    }

    #[test]
    fn places_are_named_by_the_person_or_their_address() {
        assert_eq!(Place::server("https://mila.example.com").title(), "mila.example.com");
        assert_eq!(Place { id: LOCAL.into(), name: None, port: None, opened: None }.title(), "On this computer");
        assert_eq!(Place { id: "local-ab".into(), name: Some("  Side project ".into()), port: None, opened: None }.title(), "Side project");
        assert_eq!(tidy_name("  Work\n "), Some("Work".into()));
        assert_eq!(tidy_name("   "), None);
        assert_eq!(tidy_name(&"x".repeat(100)).unwrap().len(), 60);
    }

    #[test]
    fn each_place_on_this_computer_has_its_own_folder() {
        let home = Path::new("/home/x/.local/share/mila");
        assert_eq!(local_dir(home, LOCAL), home);
        assert_eq!(local_dir(home, "local-ab12"), home.join("places").join("local-ab12"));
        let mut places = Places::default();
        assert_eq!(new_local_id(&places), LOCAL);
        places.list.push(Place { id: LOCAL.into(), name: None, port: None, opened: None });
        let next = new_local_id(&places);
        assert!(next.starts_with("local-") && next.len() > 6 && !next.contains(['/', '.']), "{next}");
    }

    #[test]
    fn deleting_a_places_work_keeps_everything_else() {
        let home = std::env::temp_dir().join(format!("mila-del-{}", crate::signin::random_token(6)));
        std::fs::create_dir_all(home.join("data")).unwrap();
        std::fs::write(home.join("data").join("mila.db"), b"x").unwrap();
        std::fs::write(home.join("server.log"), b"x").unwrap();
        std::fs::write(file(&home), b"{}").unwrap();
        let other = local_dir(&home, "local-ab12");
        std::fs::create_dir_all(other.join("data")).unwrap();
        delete_local(&home, LOCAL).unwrap();
        assert!(!home.join("data").exists() && !home.join("server.log").exists());
        assert!(file(&home).exists(), "the list of places stays");
        assert!(other.exists(), "another place's work stays");
        delete_local(&home, "local-ab12").unwrap();
        assert!(!other.exists());
        for bad in ["https://a.example", "local-../..", "local-a/b"] {
            assert!(delete_local(&home, bad).is_err(), "{bad}");
        }
        std::fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn only_a_places_own_pages_stay_in_the_window() {
        let origin = "https://mila.example.com";
        let same = |u: &str| same_origin(&url::Url::parse(u).unwrap(), origin);
        assert!(same("https://mila.example.com/wiki?x=1"));
        assert!(!same("https://mila.example.com.evil.example/"));
        assert!(!same("http://mila.example.com/"));
        assert!(!same("https://example.org/"));
    }

    #[test]
    fn versions_compare_by_their_numbers() {
        assert!(is_newer("v2.2.0", "2.1.9"));
        assert!(is_newer("2.1.10", "2.1.9"));
        assert!(is_newer("v2.1.1-3-g2bb9988", "2.1.0"));
        assert!(!is_newer("v2.1.0-3-g2bb9988", "2.1.0"));
        assert!(!is_newer("2.1.0", "2.1.0"));
        assert!(!is_newer("staged", "2.1.0"));
        assert!(!is_newer("2.2.0", "0.0.0-dev"));
    }
}
