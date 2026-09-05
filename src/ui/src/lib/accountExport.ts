/**
 * Downloading the account as a file.
 *
 * Deliberately not routed through the generated client, which is the one place
 * in the app that is true. Every other call wants a parsed object to render;
 * this one wants the server's bytes, unaltered, saved under the name the server
 * chose. Going through the client would mean parsing a training history into
 * memory and re-serializing it, which is two conversions that can only lose.
 *
 * The base path still comes from the client's own config rather than a literal,
 * so there remains exactly one place that knows where the API lives.
 */

import { client } from "./api/client.gen";

/** Used when the response carries no usable Content-Disposition. */
const FALLBACK_FILENAME = "iron-temple-export.json";

/**
 * The filename the server asked for, or a sensible one.
 *
 * Only the plain `filename=` form is read, quoted or bare, because that is the
 * only form the API emits — there is nothing outside ASCII in a username and a
 * date. Anything with a path separator in it is refused rather than cleaned:
 * a filename is a name, and a header that contains a path is not answering the
 * question that was asked.
 */
export function filenameFrom(disposition: string | null): string {
  if (!disposition) return FALLBACK_FILENAME;

  const match = /filename\s*=\s*("([^"]*)"|[^;]+)/i.exec(disposition);
  const raw = (match?.[2] ?? match?.[1] ?? "").trim();
  if (!raw || raw.includes("/") || raw.includes("\\")) return FALLBACK_FILENAME;
  return raw;
}

/** Where the export lives, from whatever base the client is configured with. */
function exportUrl(): string {
  const base = client.getConfig().baseUrl ?? "";
  return `${base.replace(/\/$/, "")}/me/export`;
}

/**
 * Fetch the export and hand it to the browser as a download.
 *
 * Resolves once the save has been triggered; rejects if the server would not
 * produce the file, so the caller can say so rather than leave someone
 * believing they have a backup they do not have.
 */
export async function downloadAccountExport(): Promise<void> {
  // same-origin credentials: the session is a cookie, and the UI is served from
  // the same origin as the API in both dev (the Vite proxy) and production.
  const response = await fetch(exportUrl(), {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(`export failed with ${response.status}`);
  }

  const blob = await response.blob();
  const filename = filenameFrom(response.headers.get("Content-Disposition"));

  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    link.click();
  } finally {
    // Revoked on a later turn, for the same reason shareImage.ts does it:
    // Safari has not finished reading the URL when click() returns, and
    // revoking synchronously saves an empty file.
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }
}
