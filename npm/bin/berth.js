#!/usr/bin/env node
"use strict";

const { execFileSync } = require("child_process");
const path = require("path");
const fs = require("fs");

// Find the Go binary — check platform-specific package first, then local.
function getBinaryPath() {
  const platform = process.platform;
  const arch = process.arch;
  const platformKey = `${platform}-${arch}`;

  // Try platform-specific package first (esbuild pattern).
  try {
    const pkgBin = require.resolve(`@berthdev/berth-${platformKey}/bin/berth`);
    if (fs.existsSync(pkgBin)) {
      return pkgBin;
    }
  } catch (e) {
    // Not installed, try local binary.
  }

  // Fall back to locally downloaded binary.
  const ext = platform === "win32" ? ".exe" : "";
  const localBin = path.join(__dirname, `berth${ext}`);
  if (fs.existsSync(localBin)) {
    return localBin;
  }

  console.error("berth: binary not found. Run 'npm install' or install from https://github.com/berthdev/berth/releases");
  process.exit(1);
}

const binaryPath = getBinaryPath();
try {
  execFileSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
} catch (e) {
  process.exit(e.status || 1);
}
