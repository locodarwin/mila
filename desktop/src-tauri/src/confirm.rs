//! Asking the person something in a dialog of the system's own, which no page in the
//! window can see or answer: the window shows a server's pages too, and whatever a page
//! can click, a hostile one could click for them.

use tauri::AppHandle;

/// Whether the person says yes to deleting a place's work, in the system's own dialog.
/// No, or no dialog to ask in, is no.
pub async fn delete_work(app: &AppHandle, title: &str) -> bool {
    let question = format!("Delete the work in “{title}” for good?");
    let detail = "Its workspaces, tasks, pages and files on this computer are deleted, and this can't be undone.";
    ask(app, question, detail.to_string(), "Delete its work").await
}

#[cfg(target_os = "linux")]
async fn ask(app: &AppHandle, question: String, detail: String, yes: &'static str) -> bool {
    let (answer, heard) = std::sync::mpsc::channel::<bool>();
    let handle = app.clone();
    // GTK's windows belong to the main thread; the answer comes back on the channel.
    let shown = app.run_on_main_thread(move || {
        use gtk::prelude::*;
        use tauri::Manager;
        let parent = handle.get_webview_window("main").and_then(|w| w.gtk_window().ok());
        let dialog = gtk::MessageDialog::new(
            parent.as_ref(),
            gtk::DialogFlags::MODAL | gtk::DialogFlags::DESTROY_WITH_PARENT,
            gtk::MessageType::Warning,
            gtk::ButtonsType::None,
            &question,
        );
        dialog.set_title("Mila");
        dialog.set_secondary_text(Some(&detail));
        dialog.add_button("Cancel", gtk::ResponseType::Cancel);
        dialog.add_button(yes, gtk::ResponseType::Accept);
        dialog.set_default_response(gtk::ResponseType::Cancel);
        let answered = std::cell::Cell::new(false);
        dialog.connect_response(move |dialog, response| {
            // Closing it answers again, as a dismissal: only the first counts.
            if answered.replace(true) {
                return;
            }
            let _ = answer.send(response == gtk::ResponseType::Accept);
            dialog.close();
        });
        dialog.show_all();
    });
    if shown.is_err() {
        return false;
    }
    tauri::async_runtime::spawn_blocking(move || heard.recv().unwrap_or(false)).await.unwrap_or(false)
}

#[cfg(windows)]
async fn ask(_app: &AppHandle, question: String, detail: String, _yes: &'static str) -> bool {
    #[link(name = "user32")]
    extern "system" {
        fn MessageBoxW(owner: *mut std::ffi::c_void, text: *const u16, caption: *const u16, kind: u32) -> i32;
    }
    const MB_OKCANCEL: u32 = 0x1;
    const MB_ICONWARNING: u32 = 0x30;
    const MB_DEFBUTTON2: u32 = 0x100;
    const MB_TASKMODAL: u32 = 0x2000;
    const MB_SETFOREGROUND: u32 = 0x1_0000;
    const MB_TOPMOST: u32 = 0x4_0000;
    const IDOK: i32 = 1;
    let wide = |s: &str| s.encode_utf16().chain(Some(0)).collect::<Vec<u16>>();
    let text = wide(&format!("{question}\n\n{detail}\n\nChoose OK to delete it."));
    let caption = wide("Mila");
    // It waits for the answer, so not on a thread the window needs.
    tauri::async_runtime::spawn_blocking(move || {
        let kind = MB_OKCANCEL | MB_ICONWARNING | MB_DEFBUTTON2 | MB_TASKMODAL | MB_SETFOREGROUND | MB_TOPMOST;
        // SAFETY: both strings end in a NUL and outlive the call; no owner window.
        unsafe { MessageBoxW(std::ptr::null_mut(), text.as_ptr(), caption.as_ptr(), kind) == IDOK }
    })
    .await
    .unwrap_or(false)
}

/// Elsewhere (macOS, which is untested), AppleScript's own dialog; with none, no.
#[cfg(not(any(target_os = "linux", windows)))]
async fn ask(_app: &AppHandle, question: String, detail: String, yes: &'static str) -> bool {
    tauri::async_runtime::spawn_blocking(move || {
        let script = format!(
            "on run argv\ndisplay dialog (item 1 of argv) buttons {{\"Cancel\", \"{yes}\"}} default button \"Cancel\" cancel button \"Cancel\" with title \"Mila\" with icon caution\nend run"
        );
        std::process::Command::new("osascript")
            .arg("-e")
            .arg(script)
            .arg(format!("{question}\n\n{detail}"))
            .output()
            .is_ok_and(|out| out.status.success() && String::from_utf8_lossy(&out.stdout).contains(yes))
    })
    .await
    .unwrap_or(false)
}
