import { invoke } from "@tauri-apps/api/core";

let cachedPort: number | null = null;

function clearPortCache() {
  cachedPort = null;
}

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

type ApiErrorPayload = {
  error?: string;
  message?: string;
  details?: unknown;
};

function buildQuery(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") q.set(key, String(value));
  }
  if (!q.has("tz_offset")) q.set("tz_offset", String(new Date().getTimezoneOffset()));
  return q.toString();
}

async function parseApiError(res: Response): Promise<ApiError> {
  let payload: ApiErrorPayload | null = null;
  let fallbackMessage = `API error: ${res.status}`;

  try {
    const contentType = res.headers.get("content-type") ?? "";
    if (contentType.includes("application/json")) {
      payload = (await res.json()) as ApiErrorPayload;
    } else {
      const text = (await res.text()).trim();
      if (text) {
        fallbackMessage = text;
      }
    }
  } catch {
    // Ignore body parsing failures and fall back to the HTTP status.
  }

  return new ApiError(
    payload?.error ?? "UNKNOWN",
    payload?.message ?? fallbackMessage,
    payload?.details,
    res.status
  );
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  let retriedAfterRefresh = false;

  while (true) {
    const usedCachedPort = cachedPort !== null;
    const port = await getPort();

    try {
      const res = await fetch(`http://127.0.0.1:${port}/api/${path}`, init);
      if (!res.ok) {
        const err = await parseApiError(res);
        if (!retriedAfterRefresh && usedCachedPort) {
          clearPortCache();
          retriedAfterRefresh = true;
          continue;
        }
        throw err;
      }

      try {
        return (await res.json()) as T;
      } catch (err) {
        if (!retriedAfterRefresh && usedCachedPort) {
          clearPortCache();
          retriedAfterRefresh = true;
          continue;
        }
        throw err;
      }
    } catch (err) {
      if (!retriedAfterRefresh && usedCachedPort) {
        clearPortCache();
        retriedAfterRefresh = true;
        continue;
      }
      throw err;
    }
  }
}

export async function fetchAPI<T>(
  path: string,
  params: Record<string, string | number | undefined>,
): Promise<T> {
  return requestJSON<T>(`${path}?${buildQuery(params)}`);
}

export async function fetchRaw<T>(path: string): Promise<T> {
  return requestJSON<T>(path);
}

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public details: unknown,
    public status: number
  ) {
    super(message);
    this.name = "ApiError";
  }
}
