// The commands the app has, named so that a capability can allow each one: pages from
// a place get only those in `allow_place` (src/main.rs), and the app's own start page
// those in capabilities/start.json.
fn main() {
    tauri_build::try_build(
        tauri_build::Attributes::new().app_manifest(tauri_build::AppManifest::new().commands(&[
            "places_list",
            "place_status",
            "connect_server",
            "cancel_connect",
            "add_local",
            "open_place",
            "rename_place",
            "sign_out_place",
            "forget_place",
            "show_folder",
            "set_open_last",
            "desktop_take_sign_in",
            "desktop_notify",
            "desktop_set_shortcut",
            "smoke_report",
        ])),
    )
    .expect("building the app");
}
