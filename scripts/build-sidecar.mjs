import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { arch, platform } from "node:process";

const targets = {
  "darwin-arm64": "aarch64-apple-darwin",
  "darwin-x64": "x86_64-apple-darwin",
  "linux-x64": "x86_64-unknown-linux-gnu",
  "win32-x64": "x86_64-pc-windows-msvc",
};

const target = process.env.TAURI_ENV_TARGET_TRIPLE || targets[`${platform}-${arch}`];
if (!target) throw new Error(`Unsupported sidecar target: ${platform}-${arch}`);

const extension = target.includes("windows") ? ".exe" : "";
const output = join("src-tauri", "binaries", `agent-usage-${target}${extension}`);
mkdirSync(join("src-tauri", "binaries"), { recursive: true });

const env = {
  ...process.env,
  CGO_ENABLED: "0",
  GOOS: target.includes("apple") ? "darwin" : target.includes("windows") ? "windows" : "linux",
  GOARCH: target.includes("aarch64") ? "arm64" : "amd64",
};

execFileSync("go", ["build", "-o", output, "."], { env, stdio: "inherit" });
