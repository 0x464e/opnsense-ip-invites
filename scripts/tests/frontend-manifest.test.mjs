import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { verifyManifest, writeManifest } from "../frontend-manifest.mjs";

test("frontend checks bind artifacts to both their source commit and their exact contents", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "ip-invites-frontend-test-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  mkdirSync(join(directory, "assets"));
  writeFileSync(join(directory, "index.html"), "<h1>App</h1>");
  writeFileSync(join(directory, "assets/app.js"), 'console.log("app")');
  const sha = "a".repeat(40);
  writeManifest(directory, sha);
  assert.doesNotThrow(() => verifyManifest(directory, sha));
  assert.throws(
    () => verifyManifest(directory, "b".repeat(40)),
    /source commit/,
  );
  writeFileSync(join(directory, "assets/app.js"), 'console.log("changed")');
  assert.throws(() => verifyManifest(directory, sha), /checksum/);
  writeManifest(directory, sha);
  writeFileSync(join(directory, "extra.js"), "unexpected");
  assert.throws(() => verifyManifest(directory, sha), /checksum/);
});
