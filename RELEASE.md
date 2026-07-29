# Release Guide / 发布指南

Agent Usage uses [Semantic Versioning 2.0.0](https://semver.org/) and publishes desktop installers from annotated Git tags. This document is the permanent release contract for maintainers.

Agent Usage 遵循 [Semantic Versioning 2.0.0](https://semver.org/)，并通过带注释的 Git 标签发布桌面安装包。本文档是维护者长期遵循的发版约定。

## Version Policy / 版本规则

| Change | Version | Examples |
| --- | --- | --- |
| Breaking compatibility / 破坏兼容 | MAJOR | incompatible config or API contract, migration that cannot preserve user state |
| Backward-compatible feature / 兼容的新功能 | MINOR | new source, analytics view, setting, or API endpoint |
| Backward-compatible fix / 兼容修复 | PATCH | bug, performance, dependency, packaging, or documentation fix |

Pre-release tags use SemVer suffixes such as `v2.1.0-alpha.1`, `v2.1.0-beta.1`, and `v2.1.0-rc.1`.

## Version Source of Truth / 版本唯一来源

The release version is committed before a tag is created. These files must contain the same version without the leading `v`:

发版版本号必须先提交，再创建标签。以下文件必须保存同一个不带 `v` 的版本号：

- `package.json`
- `package-lock.json`
- `src-tauri/tauri.conf.json`
- `src-tauri/Cargo.toml`
- the `agent-usage-desktop` package entry in `src-tauri/Cargo.lock`

The Git tag is a trigger and integrity check, not a second version source. CI rejects a tag that does not match the committed manifests.

Git 标签只负责触发发布和校验完整性，不是另一套版本来源。标签与清单版本不一致时，CI 必须失败。

## Supported Release Artifacts / 官方发布产物

| Platform | Architecture | Artifact |
| --- | --- | --- |
| macOS | Apple Silicon | `Agent Usage_<version>_aarch64.dmg` |
| Windows | x64 | `Agent Usage_<version>_x64-setup.exe` |

macOS Intel and Linux are source-build targets for v2.x and are not official installer artifacts.

macOS Intel 与 Linux 在 v2.x 中仅提供源码构建方式，不属于官方安装包范围。

## Release Checklist / 发版检查

- [ ] All intended changes are committed on `main`; the worktree is clean.
- [ ] `main` is pushed and synchronized with `origin/main`.
- [ ] The five version locations contain the target version.
- [ ] `CHANGELOG.md` contains a dated, bilingual entry for the target version.
- [ ] `README.md` and `README.zh-CN.md` describe the shipped behavior and artifacts.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] `npm test` passes.
- [ ] `npm run build` passes.
- [ ] `cargo test --manifest-path src-tauri/Cargo.toml` passes.
- [ ] A local production build has been checked on the maintainer's platform.
- [ ] The GitHub About description, topics, and community URL are current.

## Prepare a Release / 准备发布

1. Select the next version according to SemVer.
2. Update every version source listed above.
3. Move the release notes out of `[Unreleased]` into a dated bilingual version section in `CHANGELOG.md`.
4. Update user-facing documentation and screenshots when behavior changed.
5. Run the complete checklist.
6. Commit and push the preparation:

```bash
git add -A
git commit -m "chore: prepare v2.0.1 release"
git push origin main
```

## Publish / 正式发布

Create an annotated tag on the verified release commit, then push only that tag:

在通过验证的发版提交上创建带注释标签，然后只推送该标签：

```bash
git tag -a v2.0.1 -m "Release v2.0.1: concise release summary"
git push origin v2.0.1
```

The `Desktop Build` workflow then:

1. validates the tag and all committed versions;
2. extracts the matching bilingual notes from `CHANGELOG.md`;
3. builds and tests the Go sidecar, frontend, and Rust layer;
4. packages macOS Apple Silicon and Windows x64 installers;
5. creates the GitHub Release only after every required build succeeds;
6. uploads all installers and marks a stable SemVer release as Latest.

If any required job fails, fix the cause and create a new version. Do not move or overwrite a published tag.

如果任何必要任务失败，应修复后发布新版本，不要移动或覆盖已经公开的标签。

## Post-release Verification / 发布后检查

- Confirm the GitHub Actions run is green.
- Confirm the Release contains both expected platform installers and bilingual notes.
- Download and launch at least the maintainer-platform artifact.
- Verify `agent-usage-desktop version` reports the release tag.
- Verify the README installer names and community links still resolve.

## Hotfixes / 紧急修复

A hotfix increments PATCH from the latest stable tag, follows the same preparation checklist, and publishes through the same workflow. Never reuse a version number or retag an existing release.

紧急修复从最新稳定版递增 PATCH，并完整执行同一套检查与发布流程。不得复用版本号，也不得重新指向已有标签。
