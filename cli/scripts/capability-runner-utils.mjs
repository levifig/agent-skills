import { randomBytes } from "node:crypto";
import { mkdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

const requiredOptions = ["client", "expected-version", "receipt"];
const safeVersionPattern = /^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$/;

export function parseRunnerArgs(argv) {
  const values = {};
  for (let index = 0; index < argv.length; index += 2) {
    const option = argv[index];
    const value = argv[index + 1];
    if (!option?.startsWith("--") || !requiredOptions.includes(option.slice(2))) throw new Error(`unknown option ${option ?? "<missing>"}`);
    const name = option.slice(2);
    if (Object.hasOwn(values, name)) throw new Error(`duplicate option --${name}`);
    if (value === undefined || value.startsWith("--")) throw new Error(`option --${name} requires a value`);
    values[name] = value;
  }
  for (const name of requiredOptions) if (!Object.hasOwn(values, name)) throw new Error(`missing required option --${name}`);
  if (values.client === "" || values.client.startsWith("-") || /[\0\r\n]/.test(values.client)) throw new Error("--client must be a safe executable name or path");
  if (!safeVersionPattern.test(values["expected-version"])) throw new Error("--expected-version must be an exact safe identity");
  if (values.receipt === "" || /[\0\r\n]/.test(values.receipt) || !values.receipt.endsWith(".json")) throw new Error("--receipt must be a safe JSON path");
  return {
    client: values.client,
    expectedVersion: values["expected-version"],
    receiptPath: resolve(values.receipt),
  };
}

export function publishReceiptIfSuccessful(receiptPath, receipt, successful) {
  if (!successful) return false;
  const directory = dirname(receiptPath);
  mkdirSync(directory, { recursive: true });
  const temporaryPath = `${receiptPath}.${process.pid}.${randomBytes(6).toString("hex")}.tmp`;
  try {
    writeFileSync(temporaryPath, `${JSON.stringify(receipt, null, 2)}\n`, { encoding: "utf8", flag: "wx", mode: 0o644 });
    renameSync(temporaryPath, receiptPath);
  } catch (error) {
    rmSync(temporaryPath, { force: true });
    throw error;
  }
  return true;
}
