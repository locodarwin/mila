// Mila in a window of its own. A thin shell: a window showing
// a server's pages or those of a copy of Mila it runs on this computer, the Places screen
// and a menu to choose between them, signing in through the browser, a shortcut and
// desktop notifications, also for a place on this computer whose window is elsewhere.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod confirm;
mod local;
mod places;
mod signin;

use places::{Place, Places, LOCAL};
use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::collections::VecDeque;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tauri::menu::{CheckMenuItemBuilder, Menu, MenuBuilder, MenuItemBuilder, SubmenuBuilder};
use tauri::webview::{DownloadEvent, NewWindowResponse, PageLoadEvent};
use tauri::{AppHandle, Manager, RunEvent, State, Webview, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_global_shortcut::{GlobalShortcutExt, ShortcutState};
use tauri_plugin_notification::NotificationExt;
use tauri_plugin_opener::OpenerExt;

const VERSION: &str = env!("CARGO_PKG_VERSION");
const RELEASES: &str = "https://github.com/locodarwin/mila/releases";
/// How often a page can send the browser somewhere: a place can't be stopped from
/// trying, but it can't fill the screen with tabs.
const OUTSIDE_GAP: Duration = Duration::from_secs(2);
/// How many notifications a place can show in a minute, and how much each can say.
const NOTICES_A_MINUTE: usize = 10;
const OUTSIDE_A_MINUTE: usize = 5;
/// How many downloads a place can start in a minute: a page can start them with no click.
const DOWNLOADS_A_MINUTE: usize = 10;
const NOTICE_TITLE: usize = 200;
const NOTICE_BODY: usize = 500;
/// How often the app asks a place on this computer, whose window is elsewhere, for news.
const WATCH_EVERY: Duration = Duration::from_secs(15);

struct App {
    home: PathBuf,
    places: Mutex<Places>,
    /// The origin the window may show: a place's, or none on the Places screen.
    current: Mutex<Option<String>>,
    /// The place on show, by its id.
    current_place: Mutex<Option<String>>,
    /// The places on this computer running now, by id: each runs from when it is first
    /// opened until the app quits, so its reminders still show while another is on show.
    local: Mutex<HashMap<String, local::Server>>,
    /// What each place's check last said ("ready", "stopped", "offline", "signin"), for
    /// the menu.
    statuses: Mutex<HashMap<String, &'static str>>,
    cancel: Mutex<Option<Arc<AtomicBool>>>,
    /// Set when the app itself is going to its start page, and used up by that one
    /// navigation: no page can send the window there.
    start_pending: AtomicBool,
    shortcut: Mutex<Option<String>>,
    /// A sign-in the server's page is about to finish: the code and secret wait here
    /// rather than in its address, where any link could put a code of its own.
    handoff: Mutex<Option<Handoff>>,
    last_outside: Mutex<VecDeque<Instant>>,
    notices: Mutex<VecDeque<Instant>>,
    /// The downloads each place started lately, by its origin.
    downloads: Mutex<HashMap<String, VecDeque<Instant>>>,
    newer: AtomicBool,
    /// The self-test, when running: "local", or a server's address to sign in to.
    smoke: Option<String>,
    /// Whether the self-test's page has been asked to report: once, since Windows says a
    /// page has loaded again each time a link out of it is refused.
    smoke_asked: AtomicBool,
    /// How many addresses the self-test's page sent outside.
    smoke_outside: std::sync::atomic::AtomicUsize,
}

struct Handoff {
    origin: String,
    code: String,
    verifier: String,
}

#[derive(serde::Serialize)]
struct HandoffView {
    code: String,
    verifier: String,
}

/// The app's own start page, whose address differs by system.
fn start_page() -> url::Url {
    let base = if cfg!(windows) { "http://tauri.localhost/" } else { "tauri://localhost/" };
    url::Url::parse(base).expect("a fixed address")
}

fn is_start_page(address: &url::Url) -> bool {
    // Exactly the app's own address: any port of that name could be anyone's.
    let own = |scheme, host| address.scheme() == scheme && address.host_str() == Some(host) && address.port().is_none();
    own("tauri", "localhost") || own("http", "tauri.localhost")
}

/// What becomes of a page's going somewhere in the window.
#[derive(Debug, PartialEq)]
enum Going {
    /// It stays in the window.
    Stay,
    /// It opens in the person's browser instead.
    Outside,
    /// It goes nowhere.
    Refused,
}

/// Only the place on show stays in the window, and the app's own start page when the
/// app itself is going there (`start_pending`, which this uses up). A place's page
/// that went there could otherwise reach the Places screen's commands.
fn going(address: &url::Url, current: Option<&str>, start_pending: &AtomicBool) -> Going {
    if address.as_str() == "about:blank" {
        // The page's first blank moment.
        return Going::Stay;
    }
    if is_start_page(address) {
        return if current.is_none() && start_pending.swap(false, Ordering::SeqCst) { Going::Stay } else { Going::Refused };
    }
    if current.is_some_and(|o| places::same_origin(address, o)) {
        Going::Stay
    } else {
        Going::Outside
    }
}

/// The Places screen's commands work only while it is on show: never while a place's
/// pages are, even if one got the window to the screen's address.
fn on_places_screen(current: &Mutex<Option<String>>) -> Result<(), String> {
    match current.lock().unwrap().as_ref() {
        None => Ok(()),
        Some(_) => Err("That's only for the app's Places screen.".into()),
    }
}

fn window(app: &AppHandle) -> Option<tauri::WebviewWindow> {
    app.get_webview_window("main")
}

/// Pages from a place may use these commands and nothing else. Linux can't tell a frame
/// from the window, so a place's own pages keep frame-ancestors 'none'.
fn allow_place(app: &AppHandle, origin: &str) {
    let mut permissions = vec!["allow-desktop-notify", "allow-desktop-set-shortcut", "allow-desktop-take-sign-in"];
    if app.state::<App>().smoke.is_some() {
        permissions.push("allow-smoke-report");
    }
    let capability = serde_json::json!({
        "identifier": format!("place-{}", origin.replace(|c: char| !c.is_ascii_alphanumeric(), "-")),
        "windows": ["main"],
        "remote": { "urls": [format!("{origin}/*")] },
        "permissions": permissions,
    });
    if let Err(e) = app.add_capability(capability.to_string()) {
        eprintln!("could not allow {origin}: {e}");
    }
}

fn build_menu(app: &AppHandle) -> tauri::Result<Menu<tauri::Wry>> {
    let state = app.state::<App>();
    let places = state.places.lock().unwrap().clone();
    let statuses = state.statuses.lock().unwrap().clone();
    let current = state.current_place.lock().unwrap().clone();
    let mut go = SubmenuBuilder::new(app, "Mila")
        .item(&MenuItemBuilder::with_id("places", "All places…").accelerator("CmdOrCtrl+Shift+P").build(app)?)
        .separator();
    for place in &places.list {
        let said = match statuses.get(&place.id).copied() {
            Some("offline") => " (offline)",
            Some("signin") => " (sign in again)",
            _ => "",
        };
        let item = CheckMenuItemBuilder::with_id(format!("place:{}", place.id), format!("{}{said}", place.title()))
            .checked(current.as_deref() == Some(place.id.as_str()))
            .build(app)?;
        go = go.item(&item);
    }
    go = go
        .separator()
        .item(&MenuItemBuilder::with_id("add-server", "Connect to a server…").build(app)?)
        .item(&MenuItemBuilder::with_id("add-local", "New place on this computer…").build(app)?);
    if state.newer.load(Ordering::SeqCst) {
        go = go.separator().item(&MenuItemBuilder::with_id("newer", "A newer version of this app exists…").build(app)?);
    }
    go = go.separator().item(&MenuItemBuilder::with_id("about", format!("Version {VERSION}")).enabled(false).build(app)?).quit();
    MenuBuilder::new(app).item(&go.build()?).build()
}

fn refresh_menu(app: &AppHandle) {
    if let Ok(menu) = build_menu(app) {
        let _ = app.set_menu(menu);
    }
}

/// The menu made again from any thread: menus belong to the main one.
fn refresh_menu_soon(app: &AppHandle) {
    let inner = app.clone();
    let _ = app.run_on_main_thread(move || refresh_menu(&inner));
}

fn now_seconds() -> u64 {
    std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0)
}

/// Notes a place as the one on show and the one to come back to.
fn mark_opened(app: &AppHandle, id: &str) {
    let state = app.state::<App>();
    {
        let mut places = state.places.lock().unwrap();
        if let Some(place) = places.list.iter_mut().find(|p| p.id == id) {
            place.opened = Some(now_seconds());
        }
        places.last = Some(id.to_string());
    }
    *state.current_place.lock().unwrap() = Some(id.to_string());
    state.statuses.lock().unwrap().insert(id.to_string(), "ready");
    save_places(app);
    refresh_menu_soon(app);
}

fn save_places(app: &AppHandle) {
    let state = app.state::<App>();
    let places = state.places.lock().unwrap().clone();
    if let Err(e) = places::save(&state.home, &places) {
        eprintln!("could not save where things are: {e}");
    }
}

/// Lets go of the shortcut when the place on show changes: it belongs to that place's
/// pages, and the next place's pages ask for their own.
fn drop_shortcut(app: &AppHandle) {
    if let Some(old) = app.state::<App>().shortcut.lock().unwrap().take() {
        let _ = app.global_shortcut().unregister(old.as_str());
    }
}

/// Opens an address in the person's browser, which is where anything but the place on
/// show belongs. It is the page that asks, with or without a click, so not too often.
fn open_outside(app: &AppHandle, address: &url::Url) {
    if !matches!(address.scheme(), "http" | "https") {
        return;
    }
    let state = app.state::<App>();
    if state.smoke.is_some() {
        smoke_say(&state.home, &format!("sent outside: {address}"));
        state.smoke_outside.fetch_add(1, Ordering::SeqCst);
        return;
    }
    let now = Instant::now();
    let mut sent = state.last_outside.lock().unwrap();
    // Two seconds apart, and a few a minute: a page that asks with no click could
    // otherwise keep opening the browser, which has the person's own sign-ins.
    if sent.back().is_some_and(|t| now.duration_since(*t) < OUTSIDE_GAP) || !recent_allowed(&mut sent, now, OUTSIDE_A_MINUTE) {
        return;
    }
    let _ = app.opener().open_url(address.as_str(), None::<&str>);
}

/// The Places screen, or one of its parts ("server", "local") by the address's fragment.
fn show_start(app: &AppHandle, part: Option<&str>) {
    drop_shortcut(app);
    *app.state::<App>().current.lock().unwrap() = None;
    *app.state::<App>().current_place.lock().unwrap() = None;
    refresh_menu_soon(app);
    if let Some(w) = window(app) {
        let mut address = start_page();
        address.set_fragment(part);
        app.state::<App>().start_pending.store(true, Ordering::SeqCst);
        let _ = w.navigate(address);
    }
}

fn show_server(app: &AppHandle, origin: &str) -> Result<(), String> {
    enter_server(app, origin);
    let address = url::Url::parse(origin).map_err(|e| e.to_string())?;
    window(app).ok_or("no window")?.navigate(address).map_err(|e| e.to_string())
}

/// Makes a server the place on show, before the window goes to any of its pages.
fn enter_server(app: &AppHandle, origin: &str) {
    let state = app.state::<App>();
    drop_shortcut(app);
    allow_place(app, origin);
    *state.current.lock().unwrap() = Some(origin.to_string());
    state.start_pending.store(false, Ordering::SeqCst);
    mark_opened(app, origin);
    // Whether the server has moved on from this app, without holding up the window.
    let (handle, origin) = (app.clone(), origin.to_string());
    std::thread::spawn(move || {
        if server_is_newer(&origin) && !handle.state::<App>().newer.swap(true, Ordering::SeqCst) {
            let inner = handle.clone();
            let _ = handle.run_on_main_thread(move || refresh_menu(&inner));
        }
    });
}

fn instance_of(origin: &str) -> Result<serde_json::Value, String> {
    ureq::AgentBuilder::new()
        .timeout(Duration::from_secs(10))
        .build()
        .get(&format!("{origin}/api/v1/instance"))
        .call()
        .map_err(|_| "Mila couldn't reach that address.".to_string())?
        .into_json::<serde_json::Value>()
        .map_err(|_| "That address doesn't look like a Mila server.".to_string())
}

fn server_is_newer(origin: &str) -> bool {
    instance_of(origin)
        .ok()
        .and_then(|i| i["version"].as_str().map(|v| places::is_newer(v, VERSION)))
        .unwrap_or(false)
}

/// Starts a place on this computer if it isn't running, and shows it.
fn show_local(app: &AppHandle, id: &str) -> Result<(), String> {
    let state = app.state::<App>();
    {
        // The first place on this computer may be opened before it is listed: the
        // first start's choice, and the self-test.
        let mut places = state.places.lock().unwrap();
        if !places.list.iter().any(|p| p.id == id) {
            if id != LOCAL {
                return Err("The app doesn't know that place.".into());
            }
            places.list.push(Place { id: LOCAL.into(), name: None, port: None, opened: None });
        }
    }
    let (origin, first_key) = ensure_local(app, id)?;
    drop_shortcut(app);
    allow_place(app, &origin);
    *state.current.lock().unwrap() = Some(origin.clone());
    state.start_pending.store(false, Ordering::SeqCst);
    mark_opened(app, id);
    // The key works once, and signs in the window; from then on its cookie does.
    let address = match first_key {
        Some(key) => format!("{origin}/api/v1/auth/local?key={key}"),
        None => format!("{origin}/"),
    };
    let address = url::Url::parse(&address).map_err(|e| e.to_string())?;
    window(app).ok_or("no window")?.navigate(address).map_err(|e| e.to_string())
}

/// A place on this computer, running: its origin, and its sign-in key when it has just
/// started and the window has yet to use it.
fn ensure_local(app: &AppHandle, id: &str) -> Result<(String, Option<String>), String> {
    let state = app.state::<App>();
    let mut running = state.local.lock().unwrap();
    if let Some(server) = running.get(id) {
        return Ok((server.origin.clone(), None));
    }
    let wanted = state.places.lock().unwrap().list.iter().find(|p| p.id == id).and_then(|p| p.port);
    // Another place's port is never taken, even when that place isn't running now.
    let taken: Vec<u16> = state.places.lock().unwrap().list.iter().filter(|p| p.id != id).filter_map(|p| p.port).collect();
    let mut port = local::free_port(wanted).map_err(|e| e.to_string())?;
    while taken.contains(&port) {
        port = local::free_port(None).map_err(|e| e.to_string())?;
    }
    let dir = places::local_dir(&state.home, id);
    places::prepare_home(&dir).map_err(|e| format!("Could not make {}: {e}", dir.display()))?;
    let server = local::start(&dir, port)?;
    let (origin, key, watch) = (server.origin.clone(), server.key.clone(), server.watch.clone());
    running.insert(id.to_string(), server);
    drop(running);
    if let Some(place) = state.places.lock().unwrap().list.iter_mut().find(|p| p.id == id) {
        place.port = Some(port);
    }
    save_places(app);
    start_watching(app, id, &origin, &watch);
    Ok((origin, Some(key)))
}

/// What the app asks a place on this computer, with its watch key.
fn watch_local(origin: &str, watch: &str, after: Option<&str>) -> Result<serde_json::Value, String> {
    let mut address = url::Url::parse(&format!("{origin}/api/v1/local/watch")).map_err(|e| e.to_string())?;
    if let Some(after) = after {
        address.query_pairs_mut().append_pair("after", after);
    }
    ureq::AgentBuilder::new()
        .timeout(Duration::from_secs(5))
        .build()
        .get(address.as_str())
        .set("X-Mila-Watch", watch)
        .call()
        .map_err(|e| e.to_string())?
        .into_json::<serde_json::Value>()
        .map_err(|e| e.to_string())
}

/// Hears a place on this computer's new notifications while it runs, and shows them
/// on the desktop while the window shows another place: its own pages show them
/// while it is on show.
fn start_watching(app: &AppHandle, id: &str, origin: &str, watch: &str) {
    let (app, id, origin, watch) = (app.clone(), id.to_string(), origin.to_string(), watch.to_string());
    std::thread::spawn(move || {
        let mut cursor: Option<String> = None;
        loop {
            // It stops when its place does: removed, or the app quitting.
            let still = app.state::<App>().local.lock().unwrap().get(&id).is_some_and(|s| s.origin == origin);
            if !still {
                return;
            }
            if let Ok(news) = watch_local(&origin, &watch, cursor.as_deref()) {
                let on_show = app.state::<App>().current_place.lock().unwrap().as_deref() == Some(id.as_str());
                let title = app.state::<App>().places.lock().unwrap().list.iter().find(|p| p.id == id).map(Place::title).unwrap_or_default();
                if cursor.is_some() && !on_show {
                    for notice in news["notices"].as_array().into_iter().flatten() {
                        let state = app.state::<App>();
                        if !notice_allowed(&mut state.notices.lock().unwrap(), Instant::now()) {
                            break;
                        }
                        let heading = clipped(notice["title"].as_str().unwrap_or_default().to_string(), NOTICE_TITLE);
                        let _ = app
                            .notification()
                            .builder()
                            .title(format!("{heading} · {title}"))
                            .body(clipped(notice["body"].as_str().unwrap_or_default().to_string(), NOTICE_BODY))
                            .show();
                    }
                }
                if let Some(next) = news["cursor"].as_str() {
                    cursor = Some(next.to_string());
                }
            }
            std::thread::sleep(WATCH_EVERY);
        }
    });
}

#[derive(serde::Serialize)]
struct PlaceView {
    id: String,
    title: String,
    local: bool,
    /// A server's address, or a place on this computer's folder.
    address: String,
    opened: Option<u64>,
    last: bool,
    running: bool,
}

#[derive(serde::Serialize)]
struct PlacesView {
    places: Vec<PlaceView>,
    open_last: bool,
    version: &'static str,
}

#[tauri::command]
fn places_list(app: State<App>) -> Result<PlacesView, String> {
    on_places_screen(&app.current)?;
    let places = app.places.lock().unwrap().clone();
    let running = app.local.lock().unwrap();
    Ok(PlacesView {
        places: places
            .list
            .iter()
            .map(|p| PlaceView {
                id: p.id.clone(),
                title: p.title(),
                local: p.is_local(),
                address: if p.is_local() { places::local_dir(&app.home, &p.id).display().to_string() } else { places::name_of(&p.id) },
                opened: p.opened,
                last: places.last.as_deref() == Some(p.id.as_str()),
                running: running.contains_key(&p.id),
            })
            .collect(),
        open_last: places.open_last,
        version: VERSION,
    })
}

#[derive(serde::Serialize)]
struct StatusView {
    /// "ready", "stopped", "offline" or "signin".
    status: &'static str,
    /// Who the window is signed in as there.
    who: Option<String>,
    workspaces: Option<u64>,
    /// Whether the server is newer than this app.
    newer: bool,
}

/// The window's own cookies for a server, as a Cookie header, so that the app can ask
/// it who is signed in without the page. They stay in this app.
fn cookies_for(app: &AppHandle, origin: &str) -> Option<String> {
    let address = url::Url::parse(origin).ok()?;
    let cookies = window(app)?.cookies_for_url(address).ok()?;
    let header: Vec<String> = cookies.iter().map(|c| format!("{}={}", c.name(), c.value())).collect();
    (!header.is_empty()).then(|| header.join("; "))
}

fn server_status(app: &AppHandle, origin: &str) -> StatusView {
    let Ok(instance) = instance_of(origin) else {
        return StatusView { status: "offline", who: None, workspaces: None, newer: false };
    };
    let newer = instance["version"].as_str().is_some_and(|v| places::is_newer(v, VERSION));
    let agent = ureq::AgentBuilder::new().timeout(Duration::from_secs(10)).build();
    let ask = |path: &str, cookie: &str| -> Option<serde_json::Value> {
        agent.get(&format!("{origin}/api/v1{path}")).set("Cookie", cookie).call().ok()?.into_json().ok()
    };
    let signed_in = cookies_for(app, origin).and_then(|cookie| {
        let state = ask("/auth/state", &cookie)?;
        let name = state["user"]["name"].as_str()?.to_string();
        let spaces = ask("/workspaces", &cookie);
        let count = spaces.as_ref().and_then(|w| w.as_array().or_else(|| w["items"].as_array())).map(|a| a.len() as u64);
        Some((name, count))
    });
    match signed_in {
        Some((who, workspaces)) => StatusView { status: "ready", who: Some(who), workspaces, newer },
        None => StatusView { status: "signin", who: None, workspaces: None, newer },
    }
}

/// How a place is, for the Places screen: asked in the background, never holding it up.
#[tauri::command]
async fn place_status(handle: AppHandle, id: String) -> Result<StatusView, String> {
    on_places_screen(&handle.state::<App>().current)?;
    let inner = handle.clone();
    let asked = id.clone();
    let view = tauri::async_runtime::spawn_blocking(move || {
        if places::is_local(&asked) {
            let running = inner.state::<App>().local.lock().unwrap().get(&asked).map(|s| (s.origin.clone(), s.watch.clone()));
            match running {
                Some((origin, watch)) => {
                    let workspaces = watch_local(&origin, &watch, None).ok().and_then(|w| w["workspaces"].as_u64());
                    StatusView { status: "ready", who: None, workspaces, newer: false }
                }
                None => StatusView { status: "stopped", who: None, workspaces: None, newer: false },
            }
        } else {
            server_status(&inner, &asked)
        }
    })
    .await
    .map_err(|e| e.to_string())?;
    handle.state::<App>().statuses.lock().unwrap().insert(id, view.status);
    refresh_menu_soon(&handle);
    Ok(view)
}

/// Signs in to a server through the browser; resolves when the window has been sent on.
#[tauri::command]
async fn connect_server(handle: AppHandle, address: String) -> Result<(), String> {
    on_places_screen(&handle.state::<App>().current)?;
    let origin = places::normalize_server(&address)?;
    let probe = origin.clone();
    let instance = tauri::async_runtime::spawn_blocking(move || instance_of(&probe))
        .await
        .map_err(|e| e.to_string())??;
    if instance["name"] != "Mila" {
        return Err("That address doesn't look like a Mila server.".into());
    }
    let pending = signin::begin().map_err(|e| format!("Couldn't get ready to sign in: {e}"))?;
    let cancel = Arc::new(AtomicBool::new(false));
    *handle.state::<App>().cancel.lock().unwrap() = Some(cancel.clone());
    handle
        .opener()
        .open_url(signin::browser_url(&origin, &pending), None::<&str>)
        .map_err(|e| format!("Couldn't open your browser: {e}"))?;
    let verifier = pending.verifier.clone();
    let server = origin.clone();
    let waited = tauri::async_runtime::spawn_blocking(move || signin::wait_for_code(&pending, &server, &cancel))
        .await
        .map_err(|e| e.to_string())??;
    let Some(code) = waited else {
        return Err("canceled".into());
    };
    {
        let state = handle.state::<App>();
        let mut places = state.places.lock().unwrap();
        if !places.list.iter().any(|p| p.id == origin) {
            places.list.push(Place::server(&origin));
        }
    }
    save_places(&handle);
    refresh_menu(&handle);
    enter_server(&handle, &origin);
    let finish = url::Url::parse(&format!("{origin}/desktop/finish")).map_err(|e| e.to_string())?;
    *handle.state::<App>().handoff.lock().unwrap() = Some(Handoff { origin, code, verifier });
    window(&handle).ok_or("no window")?.navigate(finish).map_err(|e| e.to_string())
}

/// What the server's sign-in page asks for, once, and only from that server.
#[tauri::command]
fn desktop_take_sign_in(webview: Webview, app: State<App>) -> Option<HandoffView> {
    let page = webview.url().ok()?;
    let mut held = app.handoff.lock().unwrap();
    if !held.as_ref().is_some_and(|h| places::same_origin(&page, &h.origin)) {
        return None;
    }
    held.take().map(|h| HandoffView { code: h.code, verifier: h.verifier })
}

#[tauri::command]
fn cancel_connect(app: State<App>) {
    if on_places_screen(&app.current).is_err() {
        return;
    }
    if let Some(cancel) = app.cancel.lock().unwrap().take() {
        cancel.store(true, Ordering::SeqCst);
    }
}

/// Makes a new place on this computer, named as the person said, and opens it.
#[tauri::command]
async fn add_local(handle: AppHandle, name: Option<String>) -> Result<(), String> {
    on_places_screen(&handle.state::<App>().current)?;
    let id = {
        let state = handle.state::<App>();
        let mut places = state.places.lock().unwrap();
        let id = places::new_local_id(&places);
        let name = name.as_deref().and_then(places::tidy_name);
        places.list.push(Place { id: id.clone(), name, port: None, opened: None });
        id
    };
    save_places(&handle);
    let inner = handle.clone();
    // Starting it blocks, and the window has to stay alive meanwhile.
    tauri::async_runtime::spawn_blocking(move || show_local(&inner, &id)).await.map_err(|e| e.to_string())?
}

#[tauri::command]
async fn open_place(handle: AppHandle, id: String) -> Result<(), String> {
    on_places_screen(&handle.state::<App>().current)?;
    if places::is_local(&id) {
        let inner = handle.clone();
        tauri::async_runtime::spawn_blocking(move || show_local(&inner, &id)).await.map_err(|e| e.to_string())?
    } else if handle.state::<App>().places.lock().unwrap().list.iter().any(|p| p.id == id) {
        show_server(&handle, &id)
    } else {
        Err("The app doesn't know that place.".into())
    }
}

/// Names a place, in this app only; an empty name goes back to its address.
#[tauri::command]
fn rename_place(handle: AppHandle, id: String, name: String) -> Result<String, String> {
    on_places_screen(&handle.state::<App>().current)?;
    let title = {
        let state = handle.state::<App>();
        let mut places = state.places.lock().unwrap();
        let place = places.list.iter_mut().find(|p| p.id == id).ok_or("The app doesn't know that place.")?;
        place.name = places::tidy_name(&name);
        place.title()
    };
    save_places(&handle);
    refresh_menu(&handle);
    Ok(title)
}

/// Ends the window's session at a server, as its own Sign out would; the place stays.
#[tauri::command]
async fn sign_out_place(handle: AppHandle, id: String) -> Result<(), String> {
    on_places_screen(&handle.state::<App>().current)?;
    if places::is_local(&id) {
        return Err("There is no signing in on this computer.".into());
    }
    let inner = handle.clone();
    let origin = id.clone();
    tauri::async_runtime::spawn_blocking(move || {
        let Some(cookie) = cookies_for(&inner, &origin) else { return };
        let _ = ureq::AgentBuilder::new()
            .timeout(Duration::from_secs(10))
            .build()
            .post(&format!("{origin}/api/v1/auth/sign-out"))
            .set("Cookie", &cookie)
            .set("Origin", &origin)
            .call();
    })
    .await
    .map_err(|e| e.to_string())?;
    handle.state::<App>().statuses.lock().unwrap().insert(id, "signin");
    refresh_menu_soon(&handle);
    Ok(())
}

/// Takes a place off the list. A place on this computer stops, and its work stays in
/// its folder unless `delete` is set, `typed` is the place's name, as the person typed
/// it to say they mean it, and they say yes again in a dialog of the system's own,
/// which no page can answer for them.
#[tauri::command]
async fn forget_place(handle: AppHandle, id: String, delete: bool, typed: Option<String>) -> Result<(), String> {
    on_places_screen(&handle.state::<App>().current)?;
    let title = handle.state::<App>().places.lock().unwrap().list.iter().find(|p| p.id == id).map(Place::title).ok_or("The app doesn't know that place.")?;
    if delete && (!places::is_local(&id) || typed.as_deref().map(str::trim) != Some(title.as_str())) {
        return Err(format!("Type {title} to delete its work."));
    }
    if delete && !confirm::delete_work(&handle, &title).await {
        return Err("You said no, so nothing was removed or deleted.".into());
    }
    // A place may have come on show while the dialog was open.
    on_places_screen(&handle.state::<App>().current)?;
    let state = handle.state::<App>();
    if state.current_place.lock().unwrap().as_deref() == Some(id.as_str()) {
        show_start(&handle, None);
    }
    // Stopped before its folder goes: it may be writing.
    drop(state.local.lock().unwrap().remove(&id));
    if delete {
        places::delete_local(&state.home, &id).map_err(|e| format!("Could not delete its work: {e}"))?;
    }
    {
        let mut places = state.places.lock().unwrap();
        places.list.retain(|p| p.id != id);
        if places.last.as_deref() == Some(id.as_str()) {
            places.last = None;
        }
    }
    state.statuses.lock().unwrap().remove(&id);
    save_places(&handle);
    refresh_menu_soon(&handle);
    Ok(())
}

/// Opens a place on this computer's folder in the system's file manager: only a listed
/// place's own.
#[tauri::command]
fn show_folder(handle: AppHandle, id: String) -> Result<(), String> {
    on_places_screen(&handle.state::<App>().current)?;
    let listed = handle.state::<App>().places.lock().unwrap().list.iter().any(|p| p.id == id);
    if !listed || !places::valid_local_id(&id) {
        return Err("Only a place on this computer has a folder here.".into());
    }
    let dir = places::local_dir(&handle.state::<App>().home, &id);
    handle.opener().open_path(dir.display().to_string(), None::<&str>).map_err(|e| e.to_string())
}

/// Whether the app opens the last place at once, rather than the Places screen.
#[tauri::command]
fn set_open_last(handle: AppHandle, on: bool) -> Result<(), String> {
    on_places_screen(&handle.state::<App>().current)?;
    handle.state::<App>().places.lock().unwrap().open_last = on;
    save_places(&handle);
    Ok(())
}

/// A file a page is downloading, which goes in the person's downloads folder under its
/// own name, only ever there: with no such folder set, the webview saves to wherever the
/// app was started from. The window has nowhere else to say it's done, so it notifies.
fn on_download(app: &AppHandle, event: DownloadEvent<'_>) -> bool {
    match event {
        DownloadEvent::Requested { destination, .. } => {
            if !stays_in_folder(destination) {
                return false;
            }
            let state = app.state::<App>();
            let place = state.current.lock().unwrap().clone().unwrap_or_default();
            if !download_allowed(&mut state.downloads.lock().unwrap(), &place, Instant::now()) {
                return false;
            }
            let Some(folder) = dirs::download_dir().or_else(|| dirs::home_dir().map(|h| h.join("Downloads"))) else {
                return false;
            };
            if destination.parent() != Some(folder.as_path()) {
                let Some(name) = destination.file_name().map(|n| n.to_os_string()) else {
                    return false;
                };
                if std::fs::create_dir_all(&folder).is_err() {
                    return false;
                }
                *destination = unused_path(&folder, std::path::Path::new(&name));
            }
            true
        }
        DownloadEvent::Finished { path, success, .. } => {
            let name = path.as_deref().and_then(|p| p.file_name()).map(|n| n.to_string_lossy().into_owned());
            let state = app.state::<App>();
            if state.smoke.is_some() {
                smoke_say(&state.home, &format!("downloaded {name:?}: {success}"));
            }
            let (title, body) = match (success, name) {
                (true, Some(name)) => (format!("Saved {name}"), "It's in your downloads folder."),
                (true, None) => ("Download finished".to_string(), "It's in your downloads folder."),
                (false, _) => ("A download didn't finish".to_string(), "Try again."),
            };
            // Counted with the place's own notifications, which a page could start in a loop.
            if notice_allowed(&mut state.notices.lock().unwrap(), Instant::now()) {
                let _ = app.notification().builder().title(clipped(title, NOTICE_TITLE)).body(body).show();
            }
            true
        }
        _ => true,
    }
}

/// A name in a folder that nothing has yet: "report.txt", then "report (1).txt".
fn unused_path(folder: &std::path::Path, name: &std::path::Path) -> PathBuf {
    let mut path = folder.join(name);
    let stem = name.file_stem().unwrap_or_default().to_string_lossy().into_owned();
    let ext = name.extension().map(|e| format!(".{}", e.to_string_lossy())).unwrap_or_default();
    let mut n = 1;
    while path.exists() {
        path = folder.join(format!("{stem} ({n}){ext}"));
        n += 1;
    }
    path
}

/// Whether a file's path holds nothing that climbs out of its folder.
fn stays_in_folder(path: &std::path::Path) -> bool {
    path.is_absolute() && !path.components().any(|c| matches!(c, std::path::Component::ParentDir))
}

/// Whether a place may start another download now, given those it started lately.
fn download_allowed(started: &mut HashMap<String, VecDeque<Instant>>, place: &str, now: Instant) -> bool {
    recent_allowed(started.entry(place.to_string()).or_default(), now, DOWNLOADS_A_MINUTE)
}

/// Whether another notification may show now, given those shown lately.
fn notice_allowed(shown: &mut VecDeque<Instant>, now: Instant) -> bool {
    recent_allowed(shown, now, NOTICES_A_MINUTE)
}

/// Whether one more fits in the last minute's `most`, counting it if so.
fn recent_allowed(shown: &mut VecDeque<Instant>, now: Instant, most: usize) -> bool {
    while shown.front().is_some_and(|t| now.duration_since(*t) >= Duration::from_secs(60)) {
        shown.pop_front();
    }
    if shown.len() >= most {
        return false;
    }
    shown.push_back(now);
    true
}

fn clipped(text: String, most: usize) -> String {
    text.chars().take(most).collect()
}

#[tauri::command]
fn desktop_notify(handle: AppHandle, app: State<App>, title: String, body: String) -> Result<(), String> {
    if !notice_allowed(&mut app.notices.lock().unwrap(), Instant::now()) {
        return Ok(());
    }
    let place = app.current_place.lock().unwrap().clone();
    let origin = app.current.lock().unwrap().clone();
    let title = notice_title(title, place.as_deref(), origin.as_deref());
    handle
        .notification()
        .builder()
        .title(title)
        .body(clipped(body, NOTICE_BODY))
        .show()
        .map_err(|e| e.to_string())
}

/// A notification's title, with which place it came from unless that is a place on this
/// computer: a server's page writes the words, so they must not pass for the system's,
/// this app's own or another place's. A server at localhost is a server all the same.
fn notice_title(title: String, place: Option<&str>, origin: Option<&str>) -> String {
    let title = clipped(title, NOTICE_TITLE);
    match (place, origin) {
        (Some(id), _) if places::is_local(id) => title,
        (_, Some(origin)) => format!("{title} · {}", places::name_of(origin)),
        _ => title,
    }
}

/// Keys other programs and the system hold with two modifiers, which a server's page
/// must not take from them: a terminal's copy, paste and new tab, a browser's own,
/// opening a terminal, locking the screen, the task manager, screenshots.
const HELD_KEYS: &[(&str, &str)] = &[
    ("Mod+Shift", "C"), ("Mod+Shift", "V"), ("Mod+Shift", "X"), ("Mod+Shift", "Z"),
    ("Mod+Shift", "A"), ("Mod+Shift", "T"), ("Mod+Shift", "N"), ("Mod+Shift", "W"),
    ("Mod+Shift", "Q"), ("Mod+Shift", "I"), ("Mod+Shift", "J"), ("Mod+Shift", "P"),
    ("Mod+Shift", "R"), ("Mod+Shift", "3"), ("Mod+Shift", "4"), ("Mod+Shift", "5"),
    ("Mod+Alt", "T"), ("Mod+Alt", "L"), ("Mod+Alt", "D"), ("Mod+Alt", "Delete"),
];

/// A person's key, in the app's grammar, in the one the system's shortcuts read. A
/// sequence of keys can't be a system shortcut. It must hold two modifier keys, since it
/// takes that key from every other program, and a server's page is what asks: with one,
/// a page could take paste or copy from the whole computer. For the same reason the key
/// is a letter, a digit or Space, never one that HELD_KEYS names.
fn accelerator(binding: &str) -> Option<String> {
    if binding.contains(' ') {
        return None;
    }
    let modifiers = binding.split('+').filter(|p| matches!(*p, "Mod" | "Ctrl" | "Alt" | "Shift" | "Meta")).count();
    if modifiers != 2 || modifiers == binding.split('+').count() {
        return None;
    }
    let key = binding.rsplit('+').next()?.to_uppercase();
    let plain = key == "SPACE" || (key.len() == 1 && key.chars().all(|c| c.is_ascii_alphanumeric()));
    if !plain {
        return None;
    }
    let mut held: Vec<&str> = binding.split('+').filter(|p| matches!(*p, "Mod" | "Ctrl" | "Alt" | "Shift" | "Meta")).map(|p| if p == "Ctrl" { "Mod" } else { p }).collect();
    held.sort_by_key(|p| ["Mod", "Alt", "Shift", "Meta"].iter().position(|m| m == p));
    let combo = held.join("+");
    if HELD_KEYS.iter().any(|(m, k)| *m == combo && k.eq_ignore_ascii_case(&key)) {
        return None;
    }
    let parts: Vec<String> = binding
        .split('+')
        .map(|p| match p {
            "Mod" => "CmdOrCtrl".to_string(),
            k if k.len() == 1 => k.to_uppercase(),
            k => k.to_string(),
        })
        .collect();
    Some(parts.join("+"))
}

#[tauri::command]
fn desktop_set_shortcut(handle: AppHandle, app: State<App>, binding: String) -> Result<(), String> {
    let mut held = app.shortcut.lock().unwrap();
    let keys = handle.global_shortcut();
    if let Some(old) = held.take() {
        let _ = keys.unregister(old.as_str());
    }
    if binding.is_empty() {
        return Ok(());
    }
    let wanted = accelerator(&binding).ok_or("A shortcut from any program must be one key press with two modifier keys.")?;
    keys.register(wanted.as_str()).map_err(|e| e.to_string())?;
    *held = Some(wanted);
    Ok(())
}

/// The self-test's way of hearing from the page: only ever acts when it is running.
#[tauri::command]
fn smoke_report(handle: AppHandle, app: State<App>, report: String) {
    if let Some(kind) = &app.smoke {
        smoke_say(&app.home, &format!("page says {report}"));
        let wanted = if kind == LOCAL || kind == "1" { "\"local\":true" } else { "\"local\":false" };
        let outside = app.smoke_outside.load(Ordering::SeqCst) == 3;
        if !outside {
            smoke_say(&app.home, "links to elsewhere did not all go to the browser");
        }
        let passed = outside && report.contains("\"bridge\":true") && report.contains("\"user\":true") && report.contains(wanted);
        finish_smoke(&handle, passed);
    }
}

/// What the self-test says, also to a file: a Windows app has no output of its own.
fn smoke_say(home: &std::path::Path, text: &str) {
    println!("smoke: {text}");
    use std::io::Write;
    if let Ok(mut f) = std::fs::OpenOptions::new().create(true).append(true).open(home.join("smoke.txt")) {
        let _ = writeln!(f, "{text}");
    }
}

fn finish_smoke(handle: &AppHandle, passed: bool) {
    smoke_say(&handle.state::<App>().home, if passed { "passed" } else { "FAILED" });
    // Not on this thread, which may be the window's own and has it to keep drawing.
    let handle = handle.clone();
    std::thread::spawn(move || {
        // Long enough to take a picture of it, when asked.
        if let Some(hold) = std::env::var("MILA_DESKTOP_SMOKE_HOLD").ok().and_then(|s| s.parse().ok()) {
            std::thread::sleep(Duration::from_secs(hold));
        }
        handle.state::<App>().local.lock().unwrap().clear();
        handle.exit(if passed { 0 } else { 1 });
    });
}

fn on_menu(app: &AppHandle, id: &str) {
    match id {
        "places" => show_start(app, None),
        "add-server" => show_start(app, Some("server")),
        "add-local" => show_start(app, Some("local")),
        "newer" => {
            let _ = app.opener().open_url(RELEASES, None::<&str>);
        }
        other => {
            if let Some(place) = other.strip_prefix("place:") {
                if places::is_local(place) {
                    let (inner, place) = (app.clone(), place.to_string());
                    tauri::async_runtime::spawn_blocking(move || {
                        if let Err(e) = show_local(&inner, &place) {
                            eprintln!("{e}");
                            show_start(&inner, None);
                        }
                    });
                } else {
                    let _ = show_server(app, place);
                }
            }
        }
    }
}

fn main() {
    let smoke = std::env::var("MILA_DESKTOP_SMOKE").ok();
    let smoking = smoke.is_some();
    let smoke_kind = smoke.clone();
    let home = std::env::var_os("MILA_DESKTOP_HOME").map(PathBuf::from).unwrap_or_else(places::data_home);
    if let Err(e) = places::prepare_home(&home) {
        eprintln!("could not make {}: {e}", home.display());
    }
    let loaded = places::load(&home);
    let built = tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _, _| {
            // A second launch brings this one forward.
            if let Some(w) = window(app) {
                let _ = w.unminimize();
                let _ = w.set_focus();
            }
        }))
        .plugin(tauri_plugin_window_state::Builder::default().build())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_handler(|app, _, event| {
                    if event.state() == ShortcutState::Pressed {
                        if let Some(w) = window(app) {
                            let _ = w.unminimize();
                            let _ = w.show();
                            let _ = w.set_focus();
                            let _ = w.eval("window.dispatchEvent(new CustomEvent('mila-quick-add'))");
                        }
                    }
                })
                .build(),
        )
        .manage(App {
            home,
            places: Mutex::new(loaded),
            current: Mutex::new(None),
            current_place: Mutex::new(None),
            local: Mutex::new(HashMap::new()),
            statuses: Mutex::new(HashMap::new()),
            cancel: Mutex::new(None),
            // The window's first page is the start page.
            start_pending: AtomicBool::new(true),
            shortcut: Mutex::new(None),
            handoff: Mutex::new(None),
            last_outside: Mutex::new(VecDeque::new()),
            notices: Mutex::new(VecDeque::new()),
            downloads: Mutex::new(HashMap::new()),
            newer: AtomicBool::new(false),
            smoke: smoke_kind,
            smoke_asked: AtomicBool::new(false),
            smoke_outside: Default::default(),
        })
        .invoke_handler(tauri::generate_handler![
            places_list,
            place_status,
            connect_server,
            cancel_connect,
            add_local,
            open_place,
            rename_place,
            sign_out_place,
            forget_place,
            show_folder,
            set_open_last,
            desktop_take_sign_in,
            desktop_notify,
            desktop_set_shortcut,
            smoke_report
        ])
        .setup(move |tauri_app| {
            let app = tauri_app.handle().clone();
            let nav = app.clone();
            let loaded = app.clone();
            let popups = app.clone();
            let downloads = app.clone();
            WebviewWindowBuilder::new(tauri_app, "main", WebviewUrl::App("index.html".into()))
                .title("Mila")
                .inner_size(1200.0, 800.0)
                .min_inner_size(480.0, 400.0)
                .on_navigation(move |address| {
                    // Only the place on show and the app's own page stay in the window.
                    let state = nav.state::<App>();
                    let current = state.current.lock().unwrap().clone();
                    match going(address, current.as_deref(), &state.start_pending) {
                        Going::Stay => true,
                        Going::Outside => {
                            open_outside(&nav, address);
                            false
                        }
                        Going::Refused => {
                            eprintln!("a page tried to open the Places screen");
                            false
                        }
                    }
                })
                // So that no window appears beyond the app's control. Linux's webview never
                // asks, and does nothing with such a link, so Mila's pages send those links
                // through this window's address (`keepLinksInWindow`).
                .on_new_window(move |address, _| {
                    open_outside(&popups, &address);
                    NewWindowResponse::Deny
                })
                .on_download(move |_, event| on_download(&downloads, event))
                .on_page_load(move |_, payload| {
                    // Past a sign-in's own page, the app's first page has its data.
                    let ready = payload.url().path() == "/" && payload.url().query().is_none_or(|q| !q.contains("key="));
                    let finishing = payload.url().path() == "/desktop/finish";
                    let is_place = loaded.state::<App>().current.lock().unwrap().as_ref().is_some_and(|o| places::same_origin(payload.url(), o));
                    if smoking
                        && payload.event() == PageLoadEvent::Finished
                        && is_place
                        && (ready || finishing)
                        && !loaded.state::<App>().smoke_asked.swap(true, Ordering::SeqCst)
                    {
                        smoke_say(&loaded.state::<App>().home, &format!("the page loaded at {}", payload.url()));
                        if let Some(w) = window(&loaded) {
                            // From the page itself, as Mila's own code would. A sign-in's
                            // page moves on by itself, and the rest waits for it.
                            let _ = w.eval(
                                r#"(async () => {
  const t = window.__TAURI_INTERNALS__;
  while (location.pathname.startsWith('/desktop')) await new Promise((r) => setTimeout(r, 100));
  const state = await (await fetch('/api/v1/auth/state')).json();
  const report = { bridge: !!t, local: state.local === true, user: !!state.user, sw: 'serviceWorker' in navigator };
  if (t) await t.invoke('desktop_set_shortcut', { binding: '' }).then(() => report.shortcut = true, (e) => report.shortcut = String(e));
  // The three ways out of Mila's pages, once they have drawn.
  while (!document.querySelector('nav')) await new Promise((r) => setTimeout(r, 100));
  await new Promise((r) => setTimeout(r, 500));
  const pause = () => new Promise((r) => setTimeout(r, 500));
  for (const target of ['', '_blank']) {
    const link = document.createElement('a');
    link.href = 'https://outside.invalid/' + target;
    link.target = target;
    document.body.append(link);
    link.click();
    link.remove();
    await pause();
  }
  window.open('https://outside.invalid/open', '_blank');
  await pause();
  const file = document.createElement('a');
  file.href = URL.createObjectURL(new Blob(['the self-test']));
  file.download = 'mila-self-test.txt';
  file.click();
  await new Promise((r) => setTimeout(r, 1500));
  if (t) await t.invoke('smoke_report', { report: JSON.stringify(report) });
})()"#,
                            );
                        }
                    }
                })
                .build()?;
            refresh_menu(&app);
            app.on_menu_event(|handle, event| on_menu(handle, event.id().as_ref()));
            let state = app.state::<App>();
            for place in state.places.lock().unwrap().list.iter().filter(|p| !p.is_local()) {
                allow_place(&app, &place.id);
            }
            // The Places screen shows at the start, unless the person asked for the last
            // place at once and it is still there.
            let last = {
                let places = state.places.lock().unwrap();
                places.last.clone().filter(|l| places.open_last && places.list.iter().any(|p| &p.id == l))
            };
            if let Some(kind) = smoke.clone() {
                let inner = app.clone();
                std::thread::spawn(move || {
                    let run = if kind == LOCAL || kind == "1" {
                        show_local(&inner, LOCAL)
                    } else {
                        tauri::async_runtime::block_on(connect_server(inner.clone(), kind))
                    };
                    if let Err(e) = run {
                        smoke_say(&inner.state::<App>().home, &e);
                        finish_smoke(&inner, false);
                    }
                });
                let inner = app.clone();
                std::thread::spawn(move || {
                    std::thread::sleep(Duration::from_secs(120));
                    smoke_say(&inner.state::<App>().home, "out of time");
                    finish_smoke(&inner, false);
                });
            } else {
                match last {
                    Some(id) if places::is_local(&id) => {
                        let inner = app.clone();
                        std::thread::spawn(move || {
                            if let Err(e) = show_local(&inner, &id) {
                                eprintln!("{e}");
                                show_start(&inner, None);
                            }
                        });
                    }
                    Some(origin) => {
                        let _ = show_server(&app, &origin);
                    }
                    None => {}
                }
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("starting the app");
    built.run(|handle, event| {
        if let RunEvent::Exit = event {
            // Stops the places on this computer.
            handle.state::<App>().local.lock().unwrap().clear();
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_persons_key_becomes_a_system_shortcut() {
        assert_eq!(accelerator("Mod+Shift+Space").as_deref(), Some("CmdOrCtrl+Shift+Space"));
        assert_eq!(accelerator("Mod+Alt+k").as_deref(), Some("CmdOrCtrl+Alt+K"));
        assert_eq!(accelerator("g w"), None, "a sequence can't be one");
    }

    #[test]
    fn a_system_shortcut_needs_two_modifiers_and_a_key() {
        assert_eq!(accelerator("Mod+v"), None, "that would take paste from every program");
        assert_eq!(accelerator("k"), None);
        assert_eq!(accelerator("Mod+Shift"), None);
        assert!(accelerator("Ctrl+Alt+k").is_some());
        assert_eq!(accelerator("Mod+Shift+Alt+k"), None, "two, no more");
    }

    #[test]
    fn a_system_shortcut_leaves_other_programs_their_keys() {
        assert_eq!(accelerator("Mod+Shift+v"), None, "a terminal's paste");
        assert_eq!(accelerator("Ctrl+Shift+c"), None, "a terminal's copy");
        assert_eq!(accelerator("Ctrl+Alt+t"), None, "opening a terminal");
        assert_eq!(accelerator("Mod+Alt+Delete"), None);
        assert_eq!(accelerator("Mod+Shift+Escape"), None, "only letters, digits and Space");
        assert!(accelerator("Mod+Shift+k").is_some());
    }

    #[test]
    fn a_download_stays_in_its_folder() {
        let inside = std::env::temp_dir().join("report.txt");
        assert!(stays_in_folder(&inside));
        assert!(!stays_in_folder(&std::env::temp_dir().join("..").join("report.txt")));
        assert!(!stays_in_folder(std::path::Path::new("report.txt")));
    }

    #[test]
    fn a_download_never_overwrites_a_file() {
        let dir = std::env::temp_dir().join(format!("mila-downloads-{}", signin::random_token(6)));
        std::fs::create_dir(&dir).unwrap();
        let name = std::path::Path::new("report.txt");
        assert_eq!(unused_path(&dir, name), dir.join("report.txt"));
        std::fs::write(dir.join("report.txt"), "").unwrap();
        assert_eq!(unused_path(&dir, name), dir.join("report (1).txt"));
        std::fs::write(dir.join("report (1).txt"), "").unwrap();
        assert_eq!(unused_path(&dir, name), dir.join("report (2).txt"));
        std::fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn a_place_can_only_show_so_many_notifications() {
        let mut shown = VecDeque::new();
        let start = Instant::now();
        for _ in 0..NOTICES_A_MINUTE {
            assert!(notice_allowed(&mut shown, start));
        }
        assert!(!notice_allowed(&mut shown, start));
        assert!(notice_allowed(&mut shown, start + Duration::from_secs(61)));
    }

    #[test]
    fn the_start_page_is_recognized_on_both_systems() {
        assert!(is_start_page(&url::Url::parse("tauri://localhost/index.html").unwrap()));
        assert!(is_start_page(&url::Url::parse("http://tauri.localhost/index.html").unwrap()));
        assert!(!is_start_page(&url::Url::parse("https://example.org/").unwrap()));
        assert!(!is_start_page(&url::Url::parse("http://tauri.localhost:5555/").unwrap()), "another port is anyone's");
        assert!(!is_start_page(&url::Url::parse("tauri://elsewhere/").unwrap()));
        assert!(!is_start_page(&url::Url::parse("https://tauri.localhost/").unwrap()));
        assert!(!is_start_page(&url::Url::parse("about:srcdoc").unwrap()));
    }

    #[test]
    fn a_places_page_cannot_send_the_window_to_the_start_page() {
        let at = |u: &str| url::Url::parse(u).unwrap();
        let place = Some("https://mila.example.com");
        let pending = AtomicBool::new(false);
        for start in ["tauri://localhost/", "http://tauri.localhost/", "tauri://localhost/#local"] {
            assert_eq!(going(&at(start), place, &pending), Going::Refused, "{start}");
            assert_eq!(going(&at(start), None, &pending), Going::Refused, "{start}, unasked");
        }
        // Even when the app is about to go there, not while a place is on show.
        pending.store(true, Ordering::SeqCst);
        assert_eq!(going(&at("tauri://localhost/"), place, &pending), Going::Refused);
        // The app's own going there, once.
        assert_eq!(going(&at("tauri://localhost/#server"), None, &pending), Going::Stay);
        assert_eq!(going(&at("tauri://localhost/"), None, &pending), Going::Refused, "only once");
        // The place's own pages stay, and anywhere else goes to the browser.
        assert_eq!(going(&at("https://mila.example.com/wiki"), place, &pending), Going::Stay);
        assert_eq!(going(&at("https://elsewhere.example/"), place, &pending), Going::Outside);
        assert_eq!(going(&at("about:blank"), place, &pending), Going::Stay);
    }

    #[test]
    fn the_places_screens_commands_refuse_while_a_place_is_on_show() {
        assert!(on_places_screen(&Mutex::new(None)).is_ok());
        assert!(on_places_screen(&Mutex::new(Some("https://mila.example.com".into()))).is_err());
        assert!(on_places_screen(&Mutex::new(Some("http://127.0.0.1:9000".into()))).is_err());
    }

    #[test]
    fn a_place_can_only_start_so_many_downloads() {
        let mut started = HashMap::new();
        let start = Instant::now();
        for _ in 0..DOWNLOADS_A_MINUTE {
            assert!(download_allowed(&mut started, "https://a.example", start));
        }
        assert!(!download_allowed(&mut started, "https://a.example", start));
        assert!(download_allowed(&mut started, "https://b.example", start), "each place has its own");
        assert!(download_allowed(&mut started, "https://a.example", start + Duration::from_secs(61)));
    }

    #[test]
    fn a_notification_says_which_server_it_came_from() {
        let title = |place, origin| notice_title("Due today".into(), place, origin);
        assert_eq!(title(Some("https://mila.example.com"), Some("https://mila.example.com")), "Due today · mila.example.com");
        // A server at this computer's own address is a server too.
        assert_eq!(title(Some("http://localhost:8790"), Some("http://localhost:8790")), "Due today · localhost:8790");
        assert_eq!(title(Some("http://127.0.0.1:8790"), Some("http://127.0.0.1:8790")), "Due today · 127.0.0.1:8790");
        // A place on this computer is the app's own.
        assert_eq!(title(Some("local"), Some("http://127.0.0.1:41000")), "Due today");
        assert_eq!(title(Some("local-ab12"), Some("http://127.0.0.1:41001")), "Due today");
        assert_eq!(notice_title("x".repeat(500), Some("local"), None).len(), NOTICE_TITLE);
    }
}
