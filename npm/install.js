// install.js - Downloads platform-specific Go binary on npm install.
"use strict";

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");

const VERSION = require("./package.json").version;

// Map Node.js platform/arch to Go binary names.
const PLATFORM_MAP = {
  "darwin-arm64": "berth-darwin-arm64",
  "darwin-x64": "berth-darwin-amd64",
  "linux-x64": "berth-linux-amd64",
  "linux-arm64": "berth-linux-arm64",
  "win32-x64": "berth-windows-amd64",
};

function getPlatformKey() {
  return `${process.platform}-${process.arch}`;
}

function getBinaryName() {
  const key = getPlatformKey();
  const name = PLATFORM_MAP[key];
  if (!name) {
    console.error(`berth: unsupported platform ${key}`);
    console.error(`Supported platforms: ${Object.keys(PLATFORM_MAP).join(", ")}`);
    process.exit(1);
  }
  return name;
}

// Check if a platform-specific optional dependency package is already installed.
function tryPlatformPackage() {
  const key = getPlatformKey();
  try {
    const pkgPath = require.resolve(`@berthdev/berth-${key}/bin/berth`);
    if (fs.existsSync(pkgPath)) {
      console.log(`berth: using platform package @berthdev/berth-${key}`);
      return true;
    }
  } catch (e) {
    // Not installed, fall through to download.
  }
  return false;
}

// Download a file from a URL, following redirects.
function download(url, maxRedirects) {
  if (maxRedirects === undefined) maxRedirects = 5;

  return new Promise((resolve, reject) => {
    if (maxRedirects <= 0) {
      return reject(new Error("Too many redirects"));
    }

    https.get(url, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        return resolve(download(res.headers.location, maxRedirects - 1));
      }
      if (res.statusCode !== 200) {
        return reject(new Error(`Download failed: HTTP ${res.statusCode} from ${url}`));
      }
      const chunks = [];
      res.on("data", (chunk) => chunks.push(chunk));
      res.on("end", () => resolve(Buffer.concat(chunks)));
      res.on("error", reject);
    }).on("error", reject);
  });
}

// Extract a tar.gz buffer and write the binary to destPath.
function extractTarGz(buffer, binaryName, destPath) {
  // Use the tar command to extract since it's available on macOS and Linux.
  const tmpDir = path.join(__dirname, ".tmp-install");
  const tmpTar = path.join(tmpDir, "archive.tar.gz");

  fs.mkdirSync(tmpDir, { recursive: true });
  fs.writeFileSync(tmpTar, buffer);

  try {
    execSync(`tar -xzf "${tmpTar}" -C "${tmpDir}"`, { stdio: "pipe" });

    // Find the binary in the extracted files.
    const candidates = [
      path.join(tmpDir, "berth"),
      path.join(tmpDir, binaryName),
    ];

    let found = false;
    for (const candidate of candidates) {
      if (fs.existsSync(candidate)) {
        fs.copyFileSync(candidate, destPath);
        found = true;
        break;
      }
    }

    if (!found) {
      // List the extracted files and take the first executable-looking one.
      const files = fs.readdirSync(tmpDir).filter((f) => f !== "archive.tar.gz");
      if (files.length > 0) {
        fs.copyFileSync(path.join(tmpDir, files[0]), destPath);
        found = true;
      }
    }

    if (!found) {
      throw new Error("Could not find berth binary in archive");
    }
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

// Extract a zip buffer and write the binary to destPath.
function extractZip(buffer, destPath) {
  const tmpDir = path.join(__dirname, ".tmp-install");
  const tmpZip = path.join(tmpDir, "archive.zip");

  fs.mkdirSync(tmpDir, { recursive: true });
  fs.writeFileSync(tmpZip, buffer);

  try {
    execSync(`unzip -o "${tmpZip}" -d "${tmpDir}"`, { stdio: "pipe" });

    const files = fs.readdirSync(tmpDir).filter((f) =>
      f !== "archive.zip" && f.endsWith(".exe")
    );

    if (files.length > 0) {
      fs.copyFileSync(path.join(tmpDir, files[0]), destPath);
    } else {
      throw new Error("Could not find berth.exe in archive");
    }
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

async function main() {
  // Check platform package first.
  if (tryPlatformPackage()) {
    return;
  }

  const binaryName = getBinaryName();
  const isWindows = process.platform === "win32";
  const ext = isWindows ? ".exe" : "";
  const archiveExt = isWindows ? "zip" : "tar.gz";
  const destPath = path.join(__dirname, "bin", `berth${ext}`);

  // Ensure bin directory exists.
  fs.mkdirSync(path.join(__dirname, "bin"), { recursive: true });

  const url = `https://github.com/berthdev/berth/releases/download/v${VERSION}/berth-${binaryName.replace("berth-", "")}.${archiveExt}`;
  console.log(`berth: downloading ${url}`);

  const maxRetries = 3;
  let lastError;

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      const buffer = await download(url);

      if (isWindows) {
        extractZip(buffer, destPath);
      } else {
        extractTarGz(buffer, binaryName, destPath);
      }

      // Make binary executable.
      if (!isWindows) {
        fs.chmodSync(destPath, 0o755);
      }

      console.log(`berth: installed to ${destPath}`);
      return;
    } catch (err) {
      lastError = err;
      if (attempt < maxRetries) {
        console.warn(`berth: download attempt ${attempt} failed, retrying...`);
      }
    }
  }

  console.error(`berth: failed to download binary after ${maxRetries} attempts`);
  console.error(`berth: ${lastError.message}`);
  console.error("berth: you can install manually from https://github.com/berthdev/berth/releases");
  // Exit with 0 so npm install doesn't fail — the binary just won't be available.
  process.exit(0);
}

main();
