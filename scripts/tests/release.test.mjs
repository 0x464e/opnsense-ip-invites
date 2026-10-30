import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import {
  calculateRelease,
  compareVersions,
  git,
  prepareRelease,
  resolveRelease,
  validateInputs,
} from "../release.mjs";

function repository(t) {
  const previous = process.cwd();
  const root = mkdtempSync(join(tmpdir(), "ip-invites-release-test-"));
  const bare = join(root, "origin.git");
  const work = join(root, "work");
  execFileSync("git", ["init", "--bare", "--initial-branch=master", bare], {
    stdio: "ignore",
  });
  execFileSync("git", ["init", "--initial-branch=master", work], {
    stdio: "ignore",
  });
  process.chdir(work);
  git("config", "user.name", "Release test");
  git("config", "user.email", "release-test@example.invalid");
  git("config", "tag.gpgSign", "false");
  git("remote", "add", "origin", bare);
  commit("feat: add initial application");
  git("push", "--set-upstream", "origin", "master");
  t.after(() => {
    process.chdir(previous);
    rmSync(root, { recursive: true, force: true });
  });
  return { bare, work };
}
function commit(message) {
  writeFileSync("fixture.txt", message + "\n");
  git("add", "fixture.txt");
  git("commit", "-m", message);
}
function initialTag() {
  git("tag", "-a", "v0.1.0", "-m", "Release 0.1.0\n\nInitial release.");
}

test("release inputs reject refs, commands, ambiguous retries and malformed versions", () => {
  for (const input of ["auto", "patch", "minor", "major", "0.1.0", "12.34.56"])
    assert.doesNotThrow(() => validateInputs(input));
  for (const input of [
    "$(touch bad)",
    "v0.1.0",
    "01.2.3",
    "1.2.3-beta.1",
    "1.2.3\nevil",
    "--no-git",
  ])
    assert.throws(() => validateInputs(input));
  assert.throws(() => validateInputs("patch", "v0.1.0"));
  assert.throws(() => validateInputs("auto", "master"));
  assert.throws(() => validateInputs("auto", "v1.2.3\nevil"));
  assert.equal(compareVersions("v0.10.0", "v0.9.0"), 1);
});
test("first release previews 0.1.0 without changing files, commits or tags", async (t) => {
  repository(t);
  const head = git("rev-parse", "HEAD");
  const result = await calculateRelease();
  assert.equal(result.version, "0.1.0");
  assert.equal(git("rev-parse", "HEAD"), head);
  assert.equal(git("status", "--porcelain"), "");
  assert.equal(git("tag", "--list"), "");
});
for (const [message, expected] of [
  ["fix: correct authorization expiry", "0.1.1"],
  ["feat(ui): add search", "0.2.0"],
  ["feat!: change configuration format", "0.2.0"],
  ["deps: update the release tooling", "0.1.1"],
  ["docs: clarify setup", null],
  ["ci: improve checks", null],
]) {
  test(`recommended bump for ${message}`, async (t) => {
    repository(t);
    initialTag();
    commit(message);
    const result = await calculateRelease();
    assert.equal(result.version ?? null, expected);
    assert.equal(git("tag", "--list"), "v0.1.0");
  });
}
test("breaking changes after 1.0 bump the major", async (t) => {
  repository(t);
  git("tag", "-a", "v1.2.3", "-m", "Release 1.2.3");
  commit("fix!: remove an old interface");
  assert.equal((await calculateRelease()).version, "2.0.0");
});
test("manual increments allow releasing CI-only changes; old explicit versions are rejected", async (t) => {
  repository(t);
  initialTag();
  commit("ci: improve packaging");
  assert.equal(
    (await calculateRelease({ increment: "patch" })).version,
    "0.1.1",
  );
  await assert.rejects(calculateRelease({ increment: "0.0.9" }), /newer/);
});
test("tag preparation and retry preserve release notes without pushing anything", async (t) => {
  const { bare } = repository(t);
  const result = await prepareRelease();
  assert.equal(result.git_tag, "v0.1.0");
  assert.equal(git("cat-file", "-t", "v0.1.0"), "tag");
  assert.match(readFileSync(result.notes_file, "utf8"), /initial application/);
  const retry = await prepareRelease({ existingTag: "v0.1.0" });
  assert.equal(
    readFileSync(retry.notes_file, "utf8"),
    readFileSync(result.notes_file, "utf8"),
  );
  assert.equal(
    execFileSync("git", ["--git-dir", bare, "tag", "--list"], {
      encoding: "utf8",
    }).trim(),
    "",
  );
});
test("retrying an older tag never promotes the latest channel", async (t) => {
  repository(t);
  initialTag();
  const oldHead = git("rev-parse", "HEAD");
  commit("feat: add new behavior");
  git("tag", "-a", "v0.2.0", "-m", "Release 0.2.0\n\nNew behavior.");
  git("switch", "--detach", oldHead);
  const retry = await prepareRelease({ existingTag: "v0.1.0" });
  assert.equal(retry.publish_latest, "false");
});
test("resolve is restricted to master ancestry and exact annotated tags", (t) => {
  repository(t);
  initialTag();
  const sha = git("rev-parse", "HEAD");
  assert.equal(
    resolveRelease({ sourceSha: sha, ref: "refs/heads/master" }).source_sha,
    sha,
  );
  assert.throws(
    () => resolveRelease({ ref: "refs/heads/feature/other" }),
    /master/,
  );
  git("tag", "v0.1.1");
  assert.throws(
    () => resolveRelease({ existingTag: "v0.1.1", ref: "refs/heads/master" }),
    /annotated/,
  );
  git("switch", "--orphan", "unrelated");
  commit("feat: add unrelated code");
  const unrelated = git("rev-parse", "HEAD");
  git("tag", "-a", "v9.0.0", "-m", "Unrelated tag");
  assert.throws(() =>
    resolveRelease({ sourceSha: unrelated, ref: "refs/heads/master" }),
  );
  assert.throws(() =>
    resolveRelease({ existingTag: "v9.0.0", ref: "refs/heads/master" }),
  );
});

test("no commits since the latest release is a clean no-op", async (t) => {
  repository(t);
  initialTag();
  assert.equal((await prepareRelease()).release_created, "false");
  assert.equal(git("tag", "--list"), "v0.1.0");
});

test("release-note shell syntax is preserved as text, never executed", async (t) => {
  const { work } = repository(t);
  initialTag();
  const marker = join(work, "should-not-exist");
  commit(`fix: preserve literal $(touch ${marker}) text`);
  const result = await prepareRelease();
  assert.equal(result.version, "0.1.1");
  assert.match(readFileSync(result.notes_file, "utf8"), /\$\(touch/);
  assert.equal(git("status", "--porcelain"), "");
});
