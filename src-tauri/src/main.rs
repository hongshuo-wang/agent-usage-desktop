#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod commands;
mod sidecar;
mod tray;

use sidecar::SidecarState;
use tauri::{Emitter, Listener, Manager};
use tauri_plugin_autostart::MacosLauncher;
use tauri_plugin_notification::NotificationExt;

#[cfg(test)]
mod tests {
    use super::should_alert;

    #[test]
    fn cost_alert_fires_once_per_local_day() {
        assert!(should_alert(11.0, 10.0, true, None, "2025-01-01"));
        assert!(!should_alert(
            11.0,
            10.0,
            true,
            Some("2025-01-01"),
            "2025-01-01"
        ));
        assert!(should_alert(
            11.0,
            10.0,
            true,
            Some("2025-01-01"),
            "2025-01-02"
        ));
    }

    #[test]
    fn cost_alert_respects_threshold_and_setting() {
        assert!(!should_alert(10.0, 10.0, true, None, "2025-01-01"));
        assert!(!should_alert(9.0, 10.0, true, None, "2025-01-01"));
        assert!(!should_alert(11.0, 10.0, false, None, "2025-01-01"));
    }
}

/// Decides whether to raise the daily cost alert. One alert per local day: the
/// loop polls every five minutes, so a threshold crossed in the morning would
/// otherwise notify all day long.
fn should_alert(
    cost: f64,
    threshold: f64,
    enabled: bool,
    alerted_day: Option<&str>,
    day: &str,
) -> bool {
    enabled && cost > threshold && alerted_day != Some(day)
}

/// Reads the alert keys the notifier needs. Missing keys fall back to the same
/// defaults the settings UI shows.
fn port(app: &tauri::AppHandle) -> u16 {
    app.state::<SidecarState>()
        .port
        .load(std::sync::atomic::Ordering::Relaxed)
}

/// Returns today's locally recorded cost, or None while the sidecar is not up
/// yet or the request fails.
async fn poll_today(_app: tauri::AppHandle, port: u16) -> Option<f64> {
    if port == 0 {
        return None;
    }
    let today = chrono::Local::now().format("%Y-%m-%d").to_string();
    let url = format!(
        "http://127.0.0.1:{}/api/stats?from={}&to={}",
        port, today, today
    );
    let resp = reqwest::get(&url).await.ok()?;
    let stats = resp.json::<serde_json::Value>().await.ok()?;
    stats["total_cost"].as_f64()
}

fn read_alert_settings(app: &tauri::AppHandle) -> (f64, bool) {
    let path = app.path().app_data_dir().unwrap().join("settings.json");
    let settings = std::fs::read_to_string(&path)
        .ok()
        .and_then(|data| serde_json::from_str::<serde_json::Value>(&data).ok())
        .unwrap_or_else(|| serde_json::json!({}));
    (
        settings["cost_threshold"].as_f64().unwrap_or(10.0),
        settings["notifications_enabled"].as_bool().unwrap_or(true),
    )
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_autostart::init(
            MacosLauncher::LaunchAgent,
            None,
        ))
        .plugin(tauri_plugin_notification::init())
        .manage(SidecarState::default())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // Focus existing window when second instance is launched
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .setup(|app| {
            let handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                match sidecar::start_sidecar(&handle).await {
                    Ok(port) => {
                        println!("Sidecar started on port {}", port);
                        let _ = handle.emit("sidecar-ready", port);
                    }
                    Err(e) => {
                        eprintln!("Failed to start sidecar: {}", e);
                        let _ = handle.emit("sidecar-failed", e);
                    }
                }
            });

            // Listen for sidecar crash events and auto-restart
            let restart_handle = app.handle().clone();
            app.listen("sidecar-crashed", move |_| {
                let handle = restart_handle.clone();
                tauri::async_runtime::spawn(async move {
                    eprintln!("[sidecar] attempting restart...");
                    match sidecar::restart_sidecar(&handle).await {
                        Ok(port) => println!("[sidecar] restarted on port {}", port),
                        Err(e) => eprintln!("[sidecar] restart failed: {}", e),
                    }
                });
            });

            // Create system tray
            let today_cost_item = tray::create_tray(app.handle())?;

            // Notification check loop: poll sidecar every 5 minutes
            let notify_handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                // Tracks the local day already alerted on. The loop runs every
                // five minutes, so without it an exceeded threshold re-notifies
                // all day.
                let mut alerted_day: Option<String> = None;
                loop {
                    if let Some(cost) =
                        poll_today(notify_handle.clone(), port(&notify_handle)).await
                    {
                        let _ = today_cost_item.set_text(format!("Today: ${:.2}", cost));
                        let (threshold, enabled) = read_alert_settings(&notify_handle);
                        let day = chrono::Local::now().format("%Y-%m-%d").to_string();
                        if should_alert(cost, threshold, enabled, alerted_day.as_deref(), &day) {
                            let _ = notify_handle
                                .notification()
                                .builder()
                                .title("Agent Usage Alert")
                                .body(format!(
                                    "Daily cost ${:.2} exceeds threshold ${:.2}",
                                    cost, threshold
                                ))
                                .show();
                            alerted_day = Some(day);
                        }
                    }
                    tokio::time::sleep(std::time::Duration::from_secs(300)).await;
                }
            });

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_sidecar_port,
            commands::open_external_url,
            commands::get_cost_threshold,
            commands::set_cost_threshold,
            commands::get_notifications_enabled,
            commands::set_notifications_enabled,
            commands::restart_sidecar,
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                let _ = window.hide();
                api.prevent_close();
            }
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app, event| {
            // Every graceful exit ends here — tray Quit, Cmd+Q, an Apple Event
            // quit. Only the tray menu used to kill the sidecar, so quits that
            // went around it left the Go process running and still holding the
            // database.
            // ponytail: a hard kill (SIGKILL, crash) reaches no handler and the
            // sidecar survives. Watching the parent pid from the Go side would
            // cover that if it ever matters.
            if let tauri::RunEvent::Exit = event {
                sidecar::kill_sidecar(app);
            }
        });
}
