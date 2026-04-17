#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod commands;
mod sidecar;
mod tray;

use sidecar::SidecarState;
use std::sync::atomic::AtomicU16;
use std::sync::Mutex;
use tauri::{Emitter, Listener, Manager};
use tauri_plugin_autostart::MacosLauncher;
use tauri_plugin_notification::NotificationExt;

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_autostart::init(MacosLauncher::LaunchAgent, None))
        .plugin(tauri_plugin_notification::init())
        .manage(SidecarState {
            port: AtomicU16::new(0),
            child: Mutex::new(None),
        })
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
            tray::create_tray(app.handle())?;

            // Notification check loop: poll sidecar every 5 minutes
            let notify_handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                loop {
                    tokio::time::sleep(std::time::Duration::from_secs(300)).await;
                    let state = notify_handle.state::<SidecarState>();
                    let port = state.port.load(std::sync::atomic::Ordering::Relaxed);
                    if port == 0 {
                        continue;
                    }

                    let (threshold, enabled) = {
                        let path = notify_handle
                            .path()
                            .app_data_dir()
                            .unwrap()
                            .join("settings.json");
                        let settings = std::fs::read_to_string(&path)
                            .ok()
                            .and_then(|data| serde_json::from_str::<serde_json::Value>(&data).ok())
                            .unwrap_or_else(|| serde_json::json!({}));
                        (
                            settings["cost_threshold"].as_f64().unwrap_or(10.0),
                            settings["notifications_enabled"].as_bool().unwrap_or(true),
                        )
                    };

                    if !enabled {
                        continue;
                    }

                    let today = chrono::Local::now().format("%Y-%m-%d");
                    let url = format!(
                        "http://127.0.0.1:{}/api/stats?from={}&to={}",
                        port, today, today
                    );

                    if let Ok(resp) = reqwest::get(&url).await {
                        if let Ok(stats) = resp.json::<serde_json::Value>().await {
                            if let Some(cost) = stats["total_cost"].as_f64() {
                                if cost > threshold {
                                    let _ = notify_handle
                                        .notification()
                                        .builder()
                                        .title("Agent Usage Alert")
                                        .body(format!(
                                            "Daily cost ${:.2} exceeds threshold ${:.2}",
                                            cost, threshold
                                        ))
                                        .show();
                                }
                            }
                        }
                    }
                }
            });

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_sidecar_port,
            commands::get_cost_threshold,
            commands::set_cost_threshold,
            commands::get_notifications_enabled,
            commands::set_notifications_enabled,
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                let _ = window.hide();
                api.prevent_close();
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
