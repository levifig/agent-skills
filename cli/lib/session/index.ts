/**
 * Session library — public re-exports.
 *
 * Imports from this module:
 *
 *   import {
 *     findActiveSessionForBranch,
 *     findSessionByClaudeId,
 *     resolveCurrentSession,
 *     readSessionFile,
 *     SessionFrontmatter,
 *   } from "../lib/session/index.js";
 *
 * Note: `_parseHookSessionId` is intentionally NOT re-exported. Callers must
 * use `resolveCurrentSession({ parseStdin: true })` to read hook stdin —
 * direct stdin parsing is an internal concern of the resolution chain.
 * See SPEC-032 A5.
 */

export {
  consolidateSession,
  extractJournalLines,
  getDateTimeString,
  getTimestamp,
  readSessionFile,
  writeFileAtomic,
  type SessionFrontmatter,
  type SpecFrontmatterWithBranch,
} from "./store.js";

export { findActiveSessionForBranch, findSessionByClaudeId } from "./find.js";

export {
  resolveCurrentSession,
  type ResolveCurrentSessionOptions,
  type ResolvedSession,
} from "./resolve.js";
