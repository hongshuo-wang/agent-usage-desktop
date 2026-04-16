import { invoke } from "@tauri-apps/api/core";

let cachedPort: number | null = null;

async function getPort(): Promise<number> {
  if (cachedPort) return cachedPort;
  // Retry up to 100 times (10s total) waiting for sidecar to be ready
  for (let i = 0; i < 100; i++) {
    try {
      const port = await invoke<number>("get_sidecar_port");
      if (port > 0) {
        cachedPort = port;
        return port;
      }
    } catch {
      // invoke failed, sidecar not ready yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("Sidecar not ready");
}

function buildQuery(params: {
  from: string;
  to: string;
  granularity?: string;
  source?: string;
}): string {
  const q = new URLSearchParams();
  q.set("from", params.from);
  q.set("to", params.to);
  if (params.granularity) q.set("granularity", params.granularity);
  if (params.source) q.set("source", params.source);
  q.set("tz_offset", String(new Date().getTimezoneOffset()));
  return q.toString();
}

export async function fetchAPI<T>(path: string, params: {
  from: string;
  to: string;
  granularity?: string;
  source?: string;
}): Promise<T> {
  const port = await getPort();
  const url = `http://127.0.0.1:${port}/api/${path}?${buildQuery(params)}`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json();
}

export async function getWebUIUrl(): Promise<string> {
  const port = await getPort();
  return `http://127.0.0.1:${port}`;
}
