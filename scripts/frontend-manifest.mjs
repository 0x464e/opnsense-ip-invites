import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";
import { pathToFileURL } from "node:url";

export function digestFiles(directory) {
  const result = Object.create(null);
  function visit(path) {
    for (const entry of readdirSync(path, { withFileTypes: true }).sort(
      (a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0),
    )) {
      const full = join(path, entry.name);
      if (entry.isSymbolicLink())
        throw new Error("Frontend artifacts cannot contain symlinks.");
      if (entry.isDirectory()) visit(full);
      else if (full !== join(directory, ".build-info.json"))
        result[relative(directory, full).split(sep).join("/")] = createHash(
          "sha256",
        )
          .update(readFileSync(full))
          .digest("hex");
    }
  }
  visit(directory);
  if (!result["index.html"])
    throw new Error("Build the frontend before preparing release artifacts.");
  return result;
}
export function sourceSha() {
  try {
    return execFileSync("git", ["rev-parse", "HEAD"], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
  } catch {
    return null;
  }
}
export function writeManifest(directory, sha = sourceSha()) {
  const manifest = { source_sha: sha, files: digestFiles(directory) };
  writeFileSync(
    join(directory, ".build-info.json"),
    JSON.stringify(manifest, null, 2) + "\n",
  );
}
export function verifyManifest(directory, sha = sourceSha()) {
  const manifest = JSON.parse(
    readFileSync(join(directory, ".build-info.json"), "utf8"),
  );
  if (!sha || manifest.source_sha !== sha)
    throw new Error(
      "Frontend artifact was not built from the checked-out source commit.",
    );
  if (JSON.stringify(manifest.files) !== JSON.stringify(digestFiles(directory)))
    throw new Error("Frontend artifact checksum mismatch.");
}
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const directory = resolve(process.argv[3] || "web/dist");
  try {
    if (process.argv[2] === "write") writeManifest(directory);
    else if (process.argv[2] === "verify") verifyManifest(directory);
    else throw new Error("Use write or verify.");
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
