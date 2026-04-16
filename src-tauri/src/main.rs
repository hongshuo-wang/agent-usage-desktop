#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod commands;
mod sidecar;

use sidecar::SidecarState;
use std::sync::atomic::AtomicU16;
use std::sync::Mutex;
use tauri::{Listener, Manager};

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
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
                    }
                    Err(e) => {
                        eprintln!("Failed to start sidecar: {}", e);
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

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_sidecar_port,
            commands::get_cost_threshold,
            commands::set_cost_threshold,
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                sidecar::kill_sidecar(window.app_handle());
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
