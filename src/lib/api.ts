import { invoke } from "@tauri-apps/api/core";

let sidecarPort: number | null = null;

async function getPort(): Promise<number> {
  if (sidecarPort) return sidecarPort;
  try {
    sidecarPort = await invoke<number>("get_sidecar_port");
  } catch {
    sidecarPort = 9800;
  }
  return sidecarPort;
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
