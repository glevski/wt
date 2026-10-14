// Assembles the npm package for a release into npm/dist/, ready for
// `npm publish`: the manifest stamped with the version, the launcher script,
// and the four release binaries next to it.
//
//   node npm/build.mjs <version> <dir>
//
// <dir> holds the release tarballs the workflow builds:
// worktree-<os>-<arch>.tar.gz, each containing the `worktree` binary.
import { execFileSync } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const [version, tarballs] = process.argv.slice(2);
if (!version || !tarballs || !/^\d+\.\d+\.\d+/.test(version)) {
  console.error("usage: node npm/build.mjs <version> <dir with worktree-<os>-<arch>.tar.gz>");
  process.exit(2);
}

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, "..");
const out = join(here, "dist");
const bin = join(out, "bin");

// the names bin/worktree looks for: worktree-<os>-<arch>
const targets = ["darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"];

rmSync(out, { recursive: true, force: true });
mkdirSync(bin, { recursive: true });

for (const target of targets) {
  const tarball = join(tarballs, `worktree-${target}.tar.gz`);
  if (!existsSync(tarball)) {
    throw new Error(`missing ${tarball}`);
  }
  execFileSync("tar", ["-xzf", tarball, "-C", bin, "worktree"]);
  renameSync(join(bin, "worktree"), join(bin, `worktree-${target}`));
  chmodSync(join(bin, `worktree-${target}`), 0o755);
}

copyFileSync(join(here, "bin", "worktree"), join(bin, "worktree"));
chmodSync(join(bin, "worktree"), 0o755);
copyFileSync(join(repo, "README.md"), join(out, "README.md"));
copyFileSync(join(repo, "LICENSE"), join(out, "LICENSE"));

const manifest = JSON.parse(readFileSync(join(here, "package.json"), "utf8"));
manifest.version = version; // 0.0.0 in the repo is a placeholder
writeFileSync(join(out, "package.json"), JSON.stringify(manifest, null, 2) + "\n");

console.log(`${manifest.name}@${version} is assembled in ${out}`);
console.log(`publish it with: npm publish ${out}`);
