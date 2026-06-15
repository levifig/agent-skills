#!/usr/bin/env node
/**
 * Rewrite the Loaf Homebrew formula from release archive checksums.
 */

import { readFileSync, writeFileSync } from "node:fs";

const options = parseArgs(process.argv.slice(2));
const formulaPath = required(options.formula, "--formula");
const checksumsPath = required(options.checksums, "--checksums");
const version = required(options.version, "--version");
const repo = options.repo || "levifig/loaf";

const checksums = readChecksums(checksumsPath, version);
const targets = ["darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64"];
for (const target of targets) {
  if (!checksums[target]) {
    console.error(`ERROR: checksums file missing ${target} archive.`);
    process.exit(1);
  }
}

writeFileSync(formulaPath, formula(version, repo, checksums));
console.log(`✓ Updated ${formulaPath} for Loaf ${version}`);

function formula(versionValue, repoValue, values) {
  return `class Loaf < Formula
  desc "Opinionated agentic framework for AI coding assistants"
  homepage "https://github.com/${repoValue}"
  version "${versionValue}"
  license "MIT"

  depends_on "git"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/${repoValue}/releases/download/v#{version}/loaf_#{version}_darwin-arm64.tar.gz"
      sha256 "${values["darwin-arm64"]}"
    else
      url "https://github.com/${repoValue}/releases/download/v#{version}/loaf_#{version}_darwin-x64.tar.gz"
      sha256 "${values["darwin-x64"]}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/${repoValue}/releases/download/v#{version}/loaf_#{version}_linux-arm64.tar.gz"
      sha256 "${values["linux-arm64"]}"
    else
      url "https://github.com/${repoValue}/releases/download/v#{version}/loaf_#{version}_linux-x64.tar.gz"
      sha256 "${values["linux-x64"]}"
    end
  end

  def install
    libexec.install "bin", "package.json", "config", "content", "dist", "plugins"
    bin.write_exec_script libexec/"bin/loaf"
  end

  test do
    output = shell_output("#{bin}/loaf --version")
    assert_match "loaf", output
    assert_match version.to_s, output
  end
end
`;
}

function readChecksums(path, versionValue) {
  const result = {};
  for (const line of readFileSync(path, "utf8").split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }
    const match = trimmed.match(/^([a-f0-9]{64})\s+loaf_(.+)_(.+)\.tar\.gz$/);
    if (!match) {
      console.error(`ERROR: invalid checksum line: ${line}`);
      process.exit(1);
    }
    if (match[2] !== versionValue) {
      console.error(`ERROR: checksum line version ${match[2]} does not match ${versionValue}`);
      process.exit(1);
    }
    result[match[3]] = match[1];
  }
  return result;
}

function parseArgs(args) {
  const parsed = {};
  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (!arg.startsWith("--")) {
      console.error(`ERROR: unexpected argument ${arg}`);
      process.exit(1);
    }
    const key = arg.slice(2);
    const value = args[i + 1];
    if (!value || value.startsWith("--")) {
      console.error(`ERROR: ${arg} requires a value`);
      process.exit(1);
    }
    parsed[key] = value;
    i++;
  }
  return parsed;
}

function required(value, name) {
  if (!value) {
    console.error(`ERROR: ${name} is required.`);
    process.exit(1);
  }
  return value;
}
