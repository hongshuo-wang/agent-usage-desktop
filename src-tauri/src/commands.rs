use crate::sidecar::SidecarState;
use std::sync::atomic::Ordering;
use tauri::{AppHandle, Manager, State};
use tauri_plugin_shell::ShellExt;

const LITELLM_PRICING_URL: &str =
    "https://cdn.jsdelivr.net/gh/BerriAI/litellm@main/model_prices_and_context_window.json";

fn read_settings(app: &tauri::AppHandle) -> serde_json::Value {
    let path = app.path().app_data_dir().unwrap().join("settings.json");
    std::fs::read_to_string(&path)
        .ok()
        .and_then(|data| serde_json::from_str(&data).ok())
        .unwrap_or_else(|| serde_json::json!({}))
}

fn write_settings(app: &tauri::AppHandle, settings: &serde_json::Value) -> Result<(), String> {
    let dir = app.path().app_data_dir().unwrap();
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let path = dir.join("settings.json");
    std::fs::write(&path, serde_json::to_string_pretty(settings).unwrap())
        .map_err(|e| e.to_string())
}

#[tauri::command]
pub fn get_sidecar_port(state: State<SidecarState>) -> u16 {
    state.port.load(Ordering::Relaxed)
}

#[tauri::command]
#[allow(deprecated)]
pub fn open_external_url(app: AppHandle, url: String) -> Result<(), String> {
    if url != LITELLM_PRICING_URL {
        return Err("external URL is not allowed".into());
    }
    app.shell()
        .open(url, None)
        .map_err(|error| error.to_string())
}

#[tauri::command]
pub fn get_cost_threshold(app: tauri::AppHandle) -> f64 {
    read_settings(&app)["cost_threshold"]
        .as_f64()
        .unwrap_or(10.0)
}

#[tauri::command]
pub fn set_cost_threshold(app: tauri::AppHandle, threshold: f64) -> Result<(), String> {
    let mut settings = read_settings(&app);
    settings["cost_threshold"] = serde_json::json!(threshold);
    write_settings(&app, &settings)
}

#[tauri::command]
pub fn get_notifications_enabled(app: tauri::AppHandle) -> bool {
    read_settings(&app)["notifications_enabled"]
        .as_bool()
        .unwrap_or(true)
}

#[tauri::command]
pub fn set_notifications_enabled(app: tauri::AppHandle, enabled: bool) -> Result<(), String> {
    let mut settings = read_settings(&app);
    settings["notifications_enabled"] = serde_json::json!(enabled);
    write_settings(&app, &settings)
}

#[tauri::command]
pub async fn restart_sidecar(app: tauri::AppHandle) -> Result<u16, String> {
    crate::sidecar::restart_sidecar(&app).await
}

#[cfg(test)]
mod tests {
    #[test]
    fn external_url_command_is_exposed() {
        let _ = super::open_external_url;
    }

    #[test]
    fn restart_sidecar_command_is_exposed() {
        let _ = super::restart_sidecar;
    }
}
