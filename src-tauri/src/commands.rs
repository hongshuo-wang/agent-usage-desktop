use crate::sidecar::SidecarState;
use serde::Serialize;
use std::sync::atomic::Ordering;
use std::{path::Path, process::Command};
use tauri::{Manager, State};

const AGENT_USAGE_SKILL_REPO: &str = "hongshuo-wang/agent-usage-desktop";
const AGENT_USAGE_SKILL_NAME: &str = "agent-usage-desktop";
const AGENT_ORDER: [&str; 4] = ["claude", "codex", "opencode", "openclaw"];

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

#[derive(Serialize)]
pub struct SkillsCliActionResult {
    command: String,
    output: String,
}

fn skills_cli_agent_name(tool: &str) -> Option<&'static str> {
    match tool {
        "claude" => Some("claude-code"),
        "codex" => Some("codex"),
        "opencode" => Some("opencode"),
        "openclaw" => Some("openclaw"),
        _ => None,
    }
}

fn resolve_agent_keys(agents: Option<Vec<String>>) -> Result<Vec<&'static str>, String> {
    let requested = match agents {
        Some(list) => {
            if list.is_empty() {
                return Err("agents are required".to_string());
            }
            list
        }
        None => return Ok(AGENT_ORDER.to_vec()),
    };

    let mut seen = Vec::with_capacity(requested.len());
    for agent in requested {
        let normalized = agent.trim().to_lowercase();
        if skills_cli_agent_name(&normalized).is_none() {
            return Err(format!("invalid agent {:?}", agent));
        }
        if !seen.iter().any(|item| item == &normalized) {
            seen.push(normalized);
        }
    }

    let resolved = AGENT_ORDER
        .iter()
        .copied()
        .filter(|agent| seen.iter().any(|item| item == agent))
        .collect::<Vec<_>>();

    if resolved.is_empty() {
        return Err("agents are required".to_string());
    }

    Ok(resolved)
}

fn build_agent_usage_skill_command_args(
    action: &str,
    agents: Option<Vec<String>>,
) -> Result<Vec<String>, String> {
    let agent_keys = resolve_agent_keys(agents)?;
    let mut args = match action {
        "install" => vec![
            "add".to_string(),
            AGENT_USAGE_SKILL_REPO.to_string(),
            "--global".to_string(),
            "--skill".to_string(),
            AGENT_USAGE_SKILL_NAME.to_string(),
        ],
        "uninstall" => vec![
            "remove".to_string(),
            AGENT_USAGE_SKILL_NAME.to_string(),
            "--global".to_string(),
        ],
        _ => return Err(format!("invalid action {action:?}")),
    };

    for agent in agent_keys {
        args.push("--agent".to_string());
        args.push(
            skills_cli_agent_name(agent)
                .expect("agent order contains only known agent keys")
                .to_string(),
        );
    }
    args.push("--yes".to_string());
    Ok(args)
}

fn run_skills_shell(args: &[String]) -> Result<SkillsCliActionResult, String> {
    #[cfg(target_os = "windows")]
    let shell = ("cmd", "/C");
    #[cfg(not(target_os = "windows"))]
    let shell = if Path::new("/bin/zsh").exists() {
        ("/bin/zsh", "-lc")
    } else {
        ("/bin/sh", "-lc")
    };

    let joined = args.join(" ");
    let full_command = format!("npx --yes skills {}", joined);

    let home = dirs::home_dir().unwrap_or_else(|| std::env::current_dir().unwrap());
    let output = Command::new(shell.0)
        .arg(shell.1)
        .arg(&full_command)
        .current_dir(home)
        .output()
        .map_err(|e| e.to_string())?;

    let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
    let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
    let message = if !stdout.is_empty() { stdout } else { stderr };

    if !output.status.success() {
        return Err(if message.is_empty() {
            format!("command failed: {}", full_command)
        } else {
            message
        });
    }

    Ok(SkillsCliActionResult {
        command: full_command,
        output: if message.is_empty() {
            "completed".to_string()
        } else {
            message.lines().next().unwrap_or("completed").to_string()
        },
    })
}

#[tauri::command]
pub fn get_sidecar_port(state: State<SidecarState>) -> u16 {
    state.port.load(Ordering::Relaxed)
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
pub fn install_agent_usage_skill(
    agents: Option<Vec<String>>,
) -> Result<SkillsCliActionResult, String> {
    let args = build_agent_usage_skill_command_args("install", agents)?;
    run_skills_shell(&args)
}

#[tauri::command]
pub fn uninstall_agent_usage_skill(
    agents: Option<Vec<String>>,
) -> Result<SkillsCliActionResult, String> {
    let args = build_agent_usage_skill_command_args("uninstall", agents)?;
    run_skills_shell(&args)
}

#[cfg(test)]
mod tests {
    use super::build_agent_usage_skill_command_args;

    #[test]
    fn install_uses_only_requested_agent() {
        let args = build_agent_usage_skill_command_args("install", Some(vec!["codex".to_string()]))
            .unwrap();
        assert_eq!(
            args,
            vec![
                "add",
                "hongshuo-wang/agent-usage-desktop",
                "--global",
                "--skill",
                "agent-usage-desktop",
                "--agent",
                "codex",
                "--yes",
            ]
        );
    }

    #[test]
    fn uninstall_uses_stable_order_for_multiple_agents() {
        let args = build_agent_usage_skill_command_args(
            "uninstall",
            Some(vec![
                "openclaw".to_string(),
                "claude".to_string(),
                "codex".to_string(),
            ]),
        )
        .unwrap();
        assert_eq!(
            args,
            vec![
                "remove",
                "agent-usage-desktop",
                "--global",
                "--agent",
                "claude-code",
                "--agent",
                "codex",
                "--agent",
                "openclaw",
                "--yes",
            ]
        );
    }

    #[test]
    fn install_rejects_empty_agent_list() {
        let err = build_agent_usage_skill_command_args("install", Some(vec![])).unwrap_err();
        assert!(err.contains("agents are required"));
    }
}
