import { copyFileSync, mkdirSync } from "node:fs";
import { verifyManifest } from "./frontend-manifest.mjs";

verifyManifest("web/dist");
mkdirSync("dist/release-files", { recursive: true });
copyFileSync("examples/config.yaml", "dist/release-files/config.example.yaml");
