import { execFileSync } from "node:child_process";
import {
  appendFileSync,
  mkdtempSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import releaseIt from "release-it";
import conventionalCommits from "conventional-changelog-conventionalcommits";

const config = JSON.parse(
  readFileSync(new URL("../.release-it.json", import.meta.url), "utf8"),
);
const core = "(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)";
const versionPattern = new RegExp(`^${core}$`);
const tagPattern = new RegExp(`^v${core}$`);
export const git = (...args) =>
  execFileSync("git", args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();

export function validateInputs(increment = "auto", existingTag = "") {
  if (
    !["auto", "patch", "minor", "major"].includes(increment) &&
    !versionPattern.test(increment)
  )
    throw new Error(
      "Version must be auto, patch, minor, major, or an explicit stable version such as 0.1.0.",
    );
  if (existingTag && !tagPattern.test(existingTag))
    throw new Error("Retry tag must be an exact stable tag such as v0.1.0.");
  if (existingTag && increment !== "auto")
    throw new Error("A retry tag cannot be combined with a version override.");
}
export function compareVersions(a, b) {
  const aa = a.replace(/^v/, "").split(".").map(BigInt);
  const bb = b.replace(/^v/, "").split(".").map(BigInt);
  for (let i = 0; i < 3; i++) {
    if (aa[i] < bb[i]) return -1;
    if (aa[i] > bb[i]) return 1;
  }
  return 0;
}
export function latestTag() {
  return (
    git("tag", "--list", "v*")
      .split("\n")
      .filter((tag) => tagPattern.test(tag))
      .sort(compareVersions)
      .at(-1) || ""
  );
}
export function resolveRelease({
  increment = "auto",
  existingTag = "",
  sourceSha = git("rev-parse", "HEAD"),
  ref = process.env.GITHUB_REF,
} = {}) {
  validateInputs(increment, existingTag);
  if (ref && ref !== "refs/heads/master")
    throw new Error("Release workflows must be dispatched from master.");
  const sha = git(
    "rev-parse",
    "--verify",
    existingTag ? `refs/tags/${existingTag}^{commit}` : `${sourceSha}^{commit}`,
  );
  if (!/^[a-f0-9]{40,64}$/.test(sha)) throw new Error("Invalid source commit.");
  git("merge-base", "--is-ancestor", sha, "refs/remotes/origin/master");
  if (
    existingTag &&
    git("cat-file", "-t", `refs/tags/${existingTag}`) !== "tag"
  )
    throw new Error("Retry requires an annotated release-it tag.");
  return { source_sha: sha, existing_tag: existingTag, increment };
}
export async function calculateRelease({
  increment = "auto",
  preview = true,
} = {}) {
  validateInputs(increment);
  const latest = latestTag();
  if (latest && git("rev-list", `${latest}..HEAD`, "--count") === "0")
    return { version: null };
  if (
    versionPattern.test(increment) &&
    latest &&
    compareVersions(increment, latest) <= 0
  )
    throw new Error(
      "A new version must be newer than all existing stable release tags.",
    );
  const plugin = structuredClone(
    config.plugins["@release-it/conventional-changelog"],
  );
  const preset = await conventionalCommits(plugin.preset);
  // Before 1.0, breaking changes and ordinary features both bump the minor.
  plugin.whatBump = (commits) => {
    const bump = preset.whatBump(commits);
    return bump?.level === 0 && (!latest || latest.startsWith("v0."))
      ? { ...bump, level: 1 }
      : bump;
  };
  return releaseIt({
    ...structuredClone(config),
    config: false,
    ci: true,
    quiet: true,
    "dry-run": preview,
    increment:
      !latest && increment === "auto"
        ? "0.1.0"
        : increment === "auto"
          ? undefined
          : increment,
    git: {
      ...config.git,
      tag: !preview,
      requireCleanWorkingDir: !preview,
      requireUpstream: !preview,
    },
    plugins: { "@release-it/conventional-changelog": plugin },
  });
}
export async function prepareRelease({
  increment = "auto",
  existingTag = "",
  preview = false,
} = {}) {
  validateInputs(increment, existingTag);
  const head = git("rev-parse", "HEAD");
  let tag = existingTag;
  let result;
  if (tag) {
    if (git("rev-parse", `refs/tags/${tag}^{commit}`) !== head)
      throw new Error(
        "Retry checkout does not match the requested release tag.",
      );
    if (git("cat-file", "-t", `refs/tags/${tag}`) !== "tag")
      throw new Error("Retry requires an annotated release-it tag.");
  } else {
    result = await calculateRelease({ increment, preview });
    if (!result.version) return { release_created: "false" };
    if (!versionPattern.test(result.version))
      throw new Error("Unexpected release version.");
    tag = `v${result.version}`;
  }
  const notes =
    !existingTag && preview
      ? result?.changelog
      : git("for-each-ref", "--format=%(contents)", `refs/tags/${tag}`).replace(
          /^Release [^\n]+\n*/,
          "",
        );
  const directory = mkdtempSync(
    join(process.env.RUNNER_TEMP || tmpdir(), "ip-invites-release-"),
  );
  const notesFile = join(directory, "release-notes.md");
  writeFileSync(
    notesFile,
    (notes?.trim() || "Initial release of opnsense-ip-invites.") + "\n",
  );
  return {
    release_created: "true",
    git_tag: tag,
    version: tag.slice(1),
    source_sha: head,
    notes_file: notesFile,
    publish_latest: String(
      !latestTag() || compareVersions(tag, latestTag()) >= 0,
    ),
  };
}
function outputs(values) {
  for (const [key, value] of Object.entries(values)) {
    if (String(value).includes("\n"))
      throw new Error("Multiline workflow output rejected.");
    if (process.env.GITHUB_OUTPUT)
      appendFileSync(process.env.GITHUB_OUTPUT, `${key}=${value}\n`);
  }
  console.log(JSON.stringify(values, null, 2));
}
async function main() {
  const command = process.argv[2] || "preview";
  const input = {
    increment: process.env.RELEASE_INCREMENT || "auto",
    existingTag: process.env.RELEASE_EXISTING_TAG || "",
  };
  if (command === "resolve")
    return outputs(
      resolveRelease({
        ...input,
        sourceSha: process.env.GITHUB_SHA || git("rev-parse", "HEAD"),
      }),
    );
  if (command === "preview") {
    const result = await prepareRelease({ ...input, preview: true });
    outputs(result);
    if (result.notes_file) console.log(readFileSync(result.notes_file, "utf8"));
    return;
  }
  if (command !== "prepare" || process.env.GITHUB_ACTIONS !== "true")
    throw new Error(
      "Tag preparation runs only in the release workflow. Use npm run release:preview locally.",
    );
  const expected = process.env.SOURCE_SHA;
  if (!expected || git("rev-parse", "HEAD") !== expected)
    throw new Error("Checkout does not match the commit that passed CI.");
  if (
    !input.existingTag &&
    git("ls-remote", "origin", "refs/heads/master").split(/\s/)[0] !== expected
  )
    throw new Error(
      "master advanced while CI ran; dispatch a new release for the updated commit.",
    );
  outputs(await prepareRelease(input));
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  main().catch((error) => {
    console.error(error.message);
    process.exitCode = 1;
  });
