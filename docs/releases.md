# CI, Conventional Commits, and releases

The default and release branch is **master**. There is one version for the entire Go + React application. The frontend package is private and is not published to npm.

## Pull requests and commits

Use Conventional Commits, for example:

```text
feat(ui): add invitation search
fix(api): preserve overlapping access grants
ci: improve release validation
```

Use squash merges with a conventional PR title, which CI validates. Direct pushes to master validate the new commit range. The initial implementation commit predates this policy and does not need to be rewritten. Configure GitHub to use PR titles as squash commit titles, and allow only squash merges if enforcing this workflow through repository settings.

The `CI` workflow runs on master pushes and PRs, and is reusable by the release workflow. It checks release tooling and conventions, Go module consistency, vet/race tests, YAML validation, frontend compilation, desktop/mobile browser tests, native production startup, and GoReleaser packaging. It also builds and runs a runtime container from the packaged binary. Checks use mocks, not a real OPNsense API or deployment credential.

Each frontend build writes a source-commit/checksum manifest. CI uploads the successful bundle, including that manifest. Packaging and release jobs download the same bundle and verify it before embedding it in the Go binaries. Release packaging does not rebuild the frontend or compile the binary again inside the runtime Dockerfile.

## One-time publishing setup

1. Create the Docker Hub repository corresponding to this GitHub repository, or set the `DOCKERHUB_IMAGE` repository variable to a different `namespace/image`.
2. Add the **DOCKER_PAT** repository secret with Docker Hub permission to push that image.
3. If the Docker Hub username differs from the GitHub repository owner, set the **DOCKERHUB_USERNAME** repository variable.

Defaults derive from the GitHub repository and owner, so forks can override the registry destination. The regular CI workflow needs none of these secrets. It never pushes images or creates a release.

GitHub publication uses the job's short-lived `GITHUB_TOKEN` with `contents: write`. No GitHub personal access token or bypass of protected-master rules is needed: the workflow creates a tag, not a version/changelog commit on master, and runs GoReleaser in the same job rather than relying on a downstream tag/release event. Configure any protected-tag rules to permit the release workflow's actor.

## Release when ready

Open **Actions → Release → Run workflow**, select **master**, and leave `version` as `auto`. The workflow tests the selected commit before performing any release mutation. It checks Docker publishing credentials before creating a tag.

- The first automatic release is **0.1.0**.
- `fix`, `perf`, `deps`, and `revert` recommend a patch.
- `feat` recommends a minor.
- Breaking changes (`!` or `BREAKING CHANGE:`) recommend a minor while the current major is zero, and a major after 1.0. Features still recommend a minor before 1.0.
- Documentation, build, CI, refactoring, test, style, and maintenance changes appear in notes but do not independently trigger an automatic bump. Breaking changes in those categories still trigger a bump.
- With no new release-triggering changes, `auto` completes without creating a release.

To release a group of CI/docs-only changes, choose `patch`, `minor`, or `major`. An explicit stable version such as `0.1.0` is also accepted. Prerelease channels are not configured in this initial release pipeline.

The workflow invokes release-it programmatically with the Conventional Changelog plugin. release-it creates an annotated `vX.Y.Z` tag locally; the workflow pushes only that tag. GoReleaser owns GitHub release creation, asset upload, and container publication. There are no competing publishers, npm releases, or automatic commits to master.

The release contains:

- Linux amd64 and arm64 native archives, with the frontend embedded.
- Example YAML, systemd unit, license, README, and documentation.
- A `checksums.txt` SHA-256 manifest.
- Multi-architecture Docker Hub images tagged `X.Y.Z`, and `latest` for the newest stable release.

The release workflow is serialized. If master advances while a new release is being validated, preparation stops and asks for a new run rather than silently switching source commits.

## Retry a failed publication

A network/registry failure can happen after a tag has been pushed. Do not delete or move that tag and do not bump another version just to recover publication.

Run **Release** from master again, set `existing_tag` to the exact annotated tag (for example `v0.1.0`), and leave `version` as `auto`. The resolver checks that it belongs to master history. CI tests that tag's exact source commit and rebuilds its frontend before publication retries.

Release notes are retained in the annotated Git tag, allowing a retry even if GitHub release creation never completed. Existing release notes are preserved and assets can be replaced. Retrying an older version does **not** move the Docker `latest` tag or GitHub's latest-release selection backwards.

No real release is triggered by pushing workflow files or merging a PR. Releases always require an explicit manual workflow run.

## Local validation

With Node 24.15+:

```sh
npm ci
npm test
npm run release:preview
```

Preview mode can inspect an uncommitted checkout, but creates no tags, commits, changelog updates, or publications. A non-preview CLI tag preparation is reserved for the release workflow. Unit/integration tests exercise real release-it behavior in disposable local Git repositories and verify that tags are never pushed.

To validate packaging with GoReleaser 2.19.0:

```sh
make frontend
goreleaser check
goreleaser release --snapshot --clean --skip=publish
python3 scripts/smoke-test.py dist/goreleaser/app_linux_amd64_v1/opnsense-ip-invites
```

Outputs are under `dist/goreleaser`; the example configuration is staged under `dist/release-files`. Snapshot mode creates local platform-suffixed container tags and never publishes a production release. `make release VERSION=...` remains a separate, local-only native archive builder.

Root npm overrides pin compatible transitive tooling patches, including a downloadable es-toolkit version; dependency updates should retain the release-tooling tests and a successful `npm ci`.

References: [release-it](https://github.com/release-it/release-it), [Conventional Changelog plugin](https://github.com/release-it/conventional-changelog), [GoReleaser Docker v2](https://goreleaser.com/customization/dockers_v2/).
