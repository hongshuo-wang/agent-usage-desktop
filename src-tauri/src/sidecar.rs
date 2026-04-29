use std::sync::atomic::{AtomicU16, Ordering};
use std::sync::Mutex;
use tauri::{Emitter, Manager};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

pub struct SidecarState {
    pub port: AtomicU16,
    pub child: Mutex<Option<CommandChild>>,
}

/// Find an available TCP port
fn find_available_port() -> u16 {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    listener.local_addr().unwrap().port()
}

/// Wait for the Go sidecar to be ready by polling /api/health
async fn wait_for_health(port: u16) -> Result<(), String> {
    let url = format!("http://127.0.0.1:{}/api/health", port);
    let client = reqwest::Client::new();
    for _ in 0..50 {
        // 50 * 100ms = 5s timeout
        if let Ok(resp) = client.get(&url).send().await {
            if resp.status().is_success() {
                return Ok(());
            }
        }
        tokio::time::sleep(std::time::Duration::from_millis(100)).await;
    }
    Err("Sidecar health check timed out".into())
}

/// Start the Go sidecar process
pub async fn start_sidecar(app: &tauri::AppHandle) -> Result<u16, String> {
    let port = find_available_port();

    // Resolve config path: ~/.config/agent-usage/config.yaml
    let home = dirs::home_dir().ok_or("Cannot find home directory")?;
    let config_dir = home.join(".config").join("agent-usage");
    let config_path = config_dir.join("config.yaml");

    // Create default config if it doesn't exist — must specify absolute storage
    // path so the Go sidecar doesn't write to a relative path inside the app bundle
    std::fs::create_dir_all(&config_dir).map_err(|e| e.to_string())?;
    if !config_path.exists() {
        let db_path = config_dir.join("agent-usage.db");
        let default_config = format!(
            "storage:\n  path: \"{}\"\n",
            db_path.to_str().unwrap().replace('\\', "/")
        );
        std::fs::write(&config_path, default_config).map_err(|e| e.to_string())?;
    }

    let sidecar_command = app
        .shell()
        .sidecar("agent-usage")
        .map_err(|e| e.to_string())?
        .args([
            "--config",
            config_path.to_str().unwrap(),
            "--port",
            &port.to_string(),
        ]);

    let (mut rx, child) = sidecar_command.spawn().map_err(|e| e.to_string())?;

    // Log sidecar output in background
    let app_handle = app.clone();
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            match event {
                CommandEvent::Stdout(line) => {
                    println!("[sidecar] {}", String::from_utf8_lossy(&line));
                }
                CommandEvent::Stderr(line) => {
                    eprintln!("[sidecar] {}", String::from_utf8_lossy(&line));
                }
                CommandEvent::Terminated(payload) => {
                    eprintln!("[sidecar] terminated: {:?}", payload);
                    let _ = app_handle.emit("sidecar-crashed", ());
                }
                _ => {}
            }
        }
    });

    // Wait for health check
    wait_for_health(port).await?;

    // Store child process handle and port
    let state = app.state::<SidecarState>();
    state.port.store(port, Ordering::Relaxed);
    *state.child.lock().unwrap() = Some(child);

    Ok(port)
}

/// Kill the sidecar process gracefully
pub fn kill_sidecar(app: &tauri::AppHandle) {
    let state = app.state::<SidecarState>();
    let mut guard = state.child.lock().unwrap();
    if let Some(child) = guard.take() {
        let _ = child.kill();
    }
}

/// Restart sidecar after crash
pub async fn restart_sidecar(app: &tauri::AppHandle) -> Result<u16, String> {
    kill_sidecar(app);
    tokio::time::sleep(std::time::Duration::from_millis(500)).await;
    start_sidecar(app).await
}
