/**
 * The app's view of the generated API client.
 *
 * Everything under generated/ is written by orval from ../../../../api/openapi.yaml
 * and is not tracked (src/ui/.gitignore). This file is, and it exists so that
 * the ~40 modules importing from "./api" name one stable path instead of two
 * generated ones — orval splits its output in two, operations in one file and
 * schema types in another, and which of those a given symbol lives in is orval's
 * business rather than a call site's.
 *
 * It also means the generated tree can be reorganised — a different mode, a
 * different layout, another generator entirely — by editing this file alone,
 * which is exactly the migration that produced it.
 *
 * `export *` rather than a named list: a hand-maintained list of 37 operations
 * and ~96 schemas is a merge conflict waiting to happen and would need editing
 * every time an endpoint is added, which is the opposite of what generating the
 * client is for.
 */

// Operations: getMe(), listSessions(), and the rest, plus their per-operation
// response unions (getMeResponse, listSessionsResponse, …).
export * from "./generated/ironTemple";

// Schema types: User, Session, ProgramSummary, RackedReport, … and the *Params
// types for operations that take query parameters.
export * from "./generated/model";
