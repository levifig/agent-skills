import {
  lstatSync,
  mkdirSync,
  readFileSync,
  readlinkSync,
  renameSync,
  rmSync,
  symlinkSync,
} from "node:fs";
import { homedir } from "node:os";
import { dirname, isAbsolute, join, resolve } from "node:path";

export function refreshDevBuildLink(launcher, options = {}) {
  const home = options.home || homedir();
  const warn = options.warn || console.warn;
  const fs = {
    lstatSync,
    mkdirSync,
    readFileSync,
    readlinkSync,
    renameSync,
    rmSync,
    symlinkSync,
    ...options.fs,
  };
  const publicLink = join(home, ".local", "bin", "loaf");
  const dataHome = resolveDevLinkDataHome(options, home);
  const pointer = join(dataHome, "loaf", "current-dev-launcher");

  if ((options.platform || process.platform) === "win32") {
    return { status: "skipped", link: publicLink, pointer };
  }

  try {
    if (!publishPointer(fs, pointer, launcher, warn)) {
      return { status: "conflict", link: publicLink, pointer };
    }
    fs.mkdirSync(dirname(publicLink), { recursive: true });
    options.beforeClaimPublic?.();
    try {
      fs.symlinkSync(resolve(pointer), publicLink);
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
      if (!publicPointsAtPointer(fs, publicLink, pointer)) {
        warn(publicConflictWarning(fs, publicLink, pointer));
        return { status: "conflict", link: publicLink, pointer };
      }
    }
    return { status: "linked", link: publicLink, pointer };
  } catch (error) {
    warn(`WARN: failed to link latest dev build (${error.code || error.message})`);
    return { status: "failed", link: publicLink, pointer, error };
  }
}

function resolveDevLinkDataHome(options, home) {
  if (options.dataHome) {
    return options.dataHome;
  }
  if (options.home) {
    return join(options.home, ".local", "share");
  }
  const xdg = (process.env.XDG_DATA_HOME || "").trim();
  if (xdg && isAbsolute(xdg)) {
    return xdg;
  }
  return join(home, ".local", "share");
}

function publishPointer(fs, pointer, launcher, warn) {
  fs.mkdirSync(dirname(pointer), { recursive: true });
  let existing;
  try {
    existing = fs.lstatSync(pointer);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (existing && !existing.isSymbolicLink()) {
    warn(`WARN: not linking latest dev build; ${pointer} is not a symlink`);
    return false;
  }
  const temporary = `${pointer}.tmp-${process.pid}`;
  fs.rmSync(temporary, { force: true });
  try {
    fs.symlinkSync(resolve(launcher), temporary);
    fs.renameSync(temporary, pointer);
  } finally {
    fs.rmSync(temporary, { force: true });
  }
  return true;
}

function publicPointsAtPointer(fs, publicLink, pointer) {
  try {
    const existing = fs.lstatSync(publicLink);
    if (!existing.isSymbolicLink()) {
      return false;
    }
    return resolve(dirname(publicLink), fs.readlinkSync(publicLink)) === resolve(pointer);
  } catch {
    return false;
  }
}

function publicConflictWarning(fs, publicLink, pointer) {
  try {
    const existing = fs.lstatSync(publicLink);
    if (!existing.isSymbolicLink()) {
      return `WARN: not linking latest dev build; ${publicLink} is not a symlink`;
    }
    if (isLoafLauncherLink(fs, publicLink)) {
      return `WARN: not linking latest dev build; ${publicLink} already points at a Loaf checkout and will not be replaced. Remove it and rebuild to install the last-build pointer at ${pointer}`;
    }
    return `WARN: not linking latest dev build; ${publicLink} is not Loaf's launcher pointer`;
  } catch {
    return `WARN: not linking latest dev build; ${publicLink} is not Loaf's launcher pointer`;
  }
}

function isLoafLauncherLink(fs, link) {
  try {
    const target = resolve(dirname(link), fs.readlinkSync(link));
    const packageRoot = dirname(dirname(target));
    const manifest = JSON.parse(fs.readFileSync(join(packageRoot, "package.json"), "utf8"));
    return manifest.name === "loaf" && target === join(packageRoot, "bin", "loaf");
  } catch {
    return false;
  }
}
