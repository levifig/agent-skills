#!/usr/bin/env node
/**
 * Classify a GitHub release tag after strict SemVer validation.
 *
 * Invalid tags fail. Only a valid SemVer identity may then be classified as a
 * recognized dev build (`+g<sha>` or a legacy timestamp patch) or a release.
 */

import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const legacyDevVersionPatchFloor = 1_000_000_000;

export function classifyReleaseTag(tag) {
  if (!tag || !String(tag).startsWith("v")) {
    return { status: "invalid", error: "Release tag must start with v." };
  }
  if (!parseUpgradeSemver(tag)) {
    return { status: "invalid", error: "Release tag is not a valid SemVer identity." };
  }
  const version = String(tag).slice(1);
  if (isDevVersion(tag)) {
    return { status: "dev", tag, version };
  }
  return { status: "release", tag, version };
}

export function parseUpgradeSemver(value) {
  value = normalizeUpgradeVersion(value);
  if (value.includes("+")) {
    const plus = value.indexOf("+");
    const build = value.slice(plus + 1);
    if (!isSemverDotSeparatedIdentifiers(build, false)) {
      return null;
    }
    value = value.slice(0, plus);
  }
  let prerelease = "";
  if (value.includes("-")) {
    const hyphen = value.indexOf("-");
    prerelease = value.slice(hyphen + 1);
    if (!isSemverDotSeparatedIdentifiers(prerelease, true)) {
      return null;
    }
    value = value.slice(0, hyphen);
  }
  const parts = value.split(".");
  if (parts.length !== 3) {
    return null;
  }
  const numbers = [];
  for (const part of parts) {
    if (!isSemverNumericIdentifier(part)) {
      return null;
    }
    numbers.push(Number(part));
  }
  return { major: numbers[0], minor: numbers[1], patch: numbers[2], prerelease };
}

export function isDevVersion(version) {
  const parsed = parseUpgradeSemver(version);
  if (!parsed) {
    return false;
  }
  if (parsed.patch >= legacyDevVersionPatchFloor) {
    return true;
  }
  const value = normalizeUpgradeVersion(version);
  const plus = value.indexOf("+");
  if (plus < 0) {
    return false;
  }
  return value.slice(plus + 1).split(".").some(isDevCommitIdentifier);
}

function isDevCommitIdentifier(identifier) {
  if (identifier.length < 8 || identifier.length > 41 || identifier[0] !== "g") {
    return false;
  }
  return /^[0-9a-f]+$/.test(identifier.slice(1));
}

function normalizeUpgradeVersion(value) {
  return String(value).trim().replace(/^v/, "");
}

function isSemverDotSeparatedIdentifiers(value, prerelease) {
  if (!value) {
    return false;
  }
  for (const identifier of value.split(".")) {
    if (!identifier || !isSemverIdentifier(identifier)) {
      return false;
    }
    if (prerelease && isSemverDigits(identifier) && !isSemverNumericIdentifier(identifier)) {
      return false;
    }
  }
  return true;
}

function isSemverNumericIdentifier(value) {
  return isSemverDigits(value) && (value.length === 1 || value[0] !== "0");
}

function isSemverIdentifier(value) {
  return value.length > 0 && /^[0-9A-Za-z-]+$/.test(value);
}

function isSemverDigits(value) {
  return value.length > 0 && /^[0-9]+$/.test(value);
}

function main(argv = process.argv.slice(2)) {
  const result = classifyReleaseTag(argv[0] || "");
  if (result.status === "invalid") {
    process.stderr.write(`${result.error}\n`);
    process.exitCode = 1;
    return result;
  }
  if (result.status === "dev") {
    process.stderr.write(`Release ceremony guardrail: ${result.tag} carries a dev build identity. Skipping the release.\n`);
  }
  process.stdout.write(`tag=${result.tag}\n`);
  process.stdout.write(`ref=refs/tags/${result.tag}\n`);
  process.stdout.write(`dev=${result.status === "dev"}\n`);
  return result;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
