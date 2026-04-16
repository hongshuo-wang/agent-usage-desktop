use crate::sidecar::SidecarState;
use std::sync::atomic::Ordering;
use tauri::{Manager, State};

#[tauri::command]
pub fn get_sidecar_port(state: State<SidecarState>) -> u16 {
    state.port.load(Ordering::Relaxed)
}

#[tauri::command]
pub fn get_cost_threshold(app: tauri::AppHandle) -> f64 {
    let path = app.path().app_data_dir().unwrap().join("settings.json");
    if let Ok(data) = std::fs::read_to_string(&path) {
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&data) {
            return v["cost_threshold"].as_f64().unwrap_or(10.0);
        }
    }
    10.0
}

#[tauri::command]
pub fn set_cost_threshold(app: tauri::AppHandle, threshold: f64) -> Result<(), String> {
    let dir = app.path().app_data_dir().unwrap();
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let path = dir.join("settings.json");
    let settings = serde_json::json!({ "cost_threshold": threshold });
    std::fs::write(&path, serde_json::to_string_pretty(&settings).unwrap())
        .map_err(|e| e.to_string())
}
