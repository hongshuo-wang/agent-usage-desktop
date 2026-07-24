use std::sync::atomic::{AtomicU16, Ordering};
use std::sync::Mutex;
use tauri::{Emitter, Manager};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

#[derive(Debug, Eq, PartialEq)]
enum TerminationAction {
    Restart,
    Ignore,
}

#[derive(Default)]
struct LifecycleState {
    next_generation: u64,
    active_generation: Option<u64>,
    controlled_stop_generation: Option<u64>,
}

impl LifecycleState {
    fn begin_start(&mut self) -> u64 {
        self.next_generation += 1;
        self.active_generation = Some(self.next_generation);
        self.next_generation
    }

    fn begin_controlled_stop(&mut self, generation: u64) -> bool {
        if self.active_generation != Some(generation) {
            return false;
        }
        self.active_generation = None;
        self.controlled_stop_generation = Some(generation);
        true
    }

    fn terminated(&mut self, generation: u64) -> TerminationAction {
        if self.controlled_stop_generation == Some(generation) {
            self.controlled_stop_generation = None;
            return TerminationAction::Ignore;
        }
        if self.active_generation == Some(generation) {
            self.active_generation = None;
            return TerminationAction::Restart;
        }
        TerminationAction::Ignore
    }
}

#[derive(Default)]
struct SidecarLifecycle {
    state: Mutex<LifecycleState>,
    restart_lock: tokio::sync::Mutex<()>,
}

struct ManagedChild {
    generation: u64,
    child: CommandChild,
}

pub struct SidecarState {
    pub port: AtomicU16,
    child: Mutex<Option<ManagedChild>>,
    lifecycle: SidecarLifecycle,
}

impl Default for SidecarState {
    fn default() -> Self {
        Self {
            port: AtomicU16::new(0),
            child: Mutex::new(None),
            lifecycle: SidecarLifecycle::default(),
        }
    }
}

fn prepare_controlled_stop<R>(
    lifecycle: &Mutex<LifecycleState>,
    generation: u64,
    detach_child: impl FnOnce() -> R,
) -> R {
    lifecycle.lock().unwrap().begin_controlled_stop(generation);
    detach_child()
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
    let state = app.state::<SidecarState>();
    let _restart_guard = state.lifecycle.restart_lock.lock().await;
    start_sidecar_unlocked(app).await
}

async fn start_sidecar_unlocked(app: &tauri::AppHandle) -> Result<u16, String> {
    let port = find_available_port();
    let generation = {
        let state = app.state::<SidecarState>();
        let generation = state.lifecycle.state.lock().unwrap().begin_start();
        generation
    };

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
                    let action = {
                        let state = app_handle.state::<SidecarState>();
                        let action = state.lifecycle.state.lock().unwrap().terminated(generation);
                        action
                    };
                    if action == TerminationAction::Restart {
                        let _ = app_handle.emit("sidecar-crashed", ());
                    }
                }
                _ => {}
            }
        }
    });

    // Store the handle before health polling so failed starts can always be cleaned up.
    let state = app.state::<SidecarState>();
    *state.child.lock().unwrap() = Some(ManagedChild { generation, child });

    if let Err(error) = wait_for_health(port).await {
        kill_generation(app, generation);
        return Err(error);
    }

    state.port.store(port, Ordering::Relaxed);

    Ok(port)
}

fn kill_generation(app: &tauri::AppHandle, generation: u64) {
    let state = app.state::<SidecarState>();
    let child = prepare_controlled_stop(&state.lifecycle.state, generation, || {
        let mut guard = state.child.lock().unwrap();
        match guard.as_ref() {
            Some(managed) if managed.generation == generation => guard.take(),
            _ => None,
        }
    });
    if let Some(managed) = child {
        state.port.store(0, Ordering::Relaxed);
        let _ = managed.child.kill();
    }
}

/// Kill the sidecar process gracefully
pub fn kill_sidecar(app: &tauri::AppHandle) {
    let state = app.state::<SidecarState>();
    let generation = state
        .child
        .lock()
        .unwrap()
        .as_ref()
        .map(|managed| managed.generation);
    if let Some(generation) = generation {
        kill_generation(app, generation);
    }
}

/// Restart sidecar after crash
pub async fn restart_sidecar(app: &tauri::AppHandle) -> Result<u16, String> {
    let state = app.state::<SidecarState>();
    let _restart_guard = state.lifecycle.restart_lock.lock().await;
    kill_sidecar(app);
    tokio::time::sleep(std::time::Duration::from_millis(500)).await;
    start_sidecar_unlocked(app).await
}

#[cfg(test)]
mod tests {
    use super::{prepare_controlled_stop, LifecycleState, SidecarLifecycle, TerminationAction};
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::{Arc, Mutex};

    #[test]
    fn unexpected_active_termination_requests_one_restart() {
        let mut state = LifecycleState::default();
        let generation = state.begin_start();

        assert_eq!(state.terminated(generation), TerminationAction::Restart);
        assert_eq!(state.terminated(generation), TerminationAction::Ignore);
    }

    #[test]
    fn controlled_and_stale_terminations_are_ignored() {
        let mut state = LifecycleState::default();
        let first = state.begin_start();
        assert!(state.begin_controlled_stop(first));
        assert_eq!(state.terminated(first), TerminationAction::Ignore);

        let second = state.begin_start();
        let third = state.begin_start();
        assert!(!state.begin_controlled_stop(second));
        assert_eq!(state.terminated(second), TerminationAction::Ignore);
        assert_eq!(state.terminated(third), TerminationAction::Restart);
    }

    #[test]
    fn controlled_stop_is_recorded_before_child_detach() {
        let lifecycle = Mutex::new(LifecycleState::default());
        let generation = lifecycle.lock().unwrap().begin_start();
        let observed = Mutex::new(None);

        prepare_controlled_stop(&lifecycle, generation, || {
            *observed.lock().unwrap() = Some(lifecycle.lock().unwrap().terminated(generation));
        });

        assert_eq!(*observed.lock().unwrap(), Some(TerminationAction::Ignore));
    }

    #[tokio::test(flavor = "current_thread")]
    async fn restart_gate_serializes_concurrent_operations() {
        let lifecycle = Arc::new(SidecarLifecycle::default());
        let active = Arc::new(AtomicUsize::new(0));
        let maximum = Arc::new(AtomicUsize::new(0));
        let mut tasks = Vec::new();
        for _ in 0..4 {
            let lifecycle = lifecycle.clone();
            let active = active.clone();
            let maximum = maximum.clone();
            tasks.push(tokio::spawn(async move {
                let _guard = lifecycle.restart_lock.lock().await;
                let current = active.fetch_add(1, Ordering::SeqCst) + 1;
                maximum.fetch_max(current, Ordering::SeqCst);
                tokio::task::yield_now().await;
                active.fetch_sub(1, Ordering::SeqCst);
            }));
        }
        for task in tasks {
            task.await.unwrap();
        }
        assert_eq!(maximum.load(Ordering::SeqCst), 1);
    }
}
