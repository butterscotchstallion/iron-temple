/**
 * Turning the Racked share card into a file, and the file into a share.
 *
 * The browser-facing half of shareCard.ts: everything here touches a canvas, a
 * blob, or `navigator`, and is kept out of the painting and layout code so that
 * those stay testable without any of it.
 */

import type { RackedPeriod, RackedReport } from "./api";
import { SHARE_CARD, font, paintShareCard, shareCardContent } from "./shareCard";

/** What happened when the file was handed off. */
export type ShareOutcome = "shared" | "cancelled" | "downloaded";

/**
 * The weights the card draws in. Inter arrives through
 * `@fontsource-variable/inter` (app.css), which registers the face but does not
 * download it until something asks for it — and canvas asking for it by setting
 * `ctx.font` is not an ask the font loader can see. Without this the first card
 * silently paints in the system sans and the second one, after the page has
 * used those weights somewhere, paints in Inter: same code, two different
 * images. Loading them up front makes it always the second case.
 */
const WEIGHTS = [400, 600, 700, 800];

async function loadFonts(): Promise<void> {
  const fonts = document.fonts;
  if (!fonts?.load) return;
  try {
    await Promise.all(WEIGHTS.map((weight) => fonts.load(font(weight, 48))));
  } catch {
    // A font that will not load is not a reason to refuse to draw the card —
    // the fallback stack still renders every glyph, just a little differently.
  }
}

/**
 * Render the recap as a PNG.
 *
 * The canvas is never attached to the document: it exists to be painted once
 * and read back as bytes, and appending it would put a 1080×1350 element behind
 * the dialog for no reason.
 */
export async function renderShareCard(
  report: RackedReport,
  displayName: string = "",
): Promise<Blob> {
  const canvas = document.createElement("canvas");
  canvas.width = SHARE_CARD.width;
  canvas.height = SHARE_CARD.height;

  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("canvas is unavailable");

  await loadFonts();
  paintShareCard(ctx, shareCardContent(report, displayName));

  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => {
      // Spec says null only on an encoding failure, but a null here would
      // otherwise become an unhandled promise that never settles.
      blob ? resolve(blob) : reject(new Error("could not encode the card"));
    }, "image/png");
  });
}

/** `racked-march-2026.png`, from the period's own label. */
export function shareCardFilename(period: RackedPeriod): string {
  const slug = period.label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
  return `racked-${slug || "recap"}.png`;
}

export function shareCardFile(blob: Blob, filename: string): File {
  return new File([blob], filename, { type: "image/png" });
}

/**
 * Whether this browser will take the image itself, rather than only text or a
 * link. Asked with the real file because `canShare` inspects it — Safari and
 * Chrome both refuse file shares for types they will not carry, and desktop
 * Firefox has no file sharing at all.
 */
export function canShareFile(file: File): boolean {
  return (
    typeof navigator !== "undefined" &&
    typeof navigator.share === "function" &&
    typeof navigator.canShare === "function" &&
    navigator.canShare({ files: [file] })
  );
}

/**
 * Hand the card to the OS share sheet, or save it.
 *
 * Dismissing the share sheet rejects with an `AbortError`. That is the user
 * changing their mind, not a failure, and reporting it as one would put an
 * error message on screen every time somebody backed out.
 */
export async function shareOrDownload(file: File): Promise<ShareOutcome> {
  if (canShareFile(file)) {
    try {
      await navigator.share({ files: [file], title: "Racked" });
      return "shared";
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") return "cancelled";
      throw err;
    }
  }

  const url = URL.createObjectURL(file);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download = file.name;
    link.click();
  } finally {
    // Revoked on a later turn: Safari has not finished reading the URL when
    // click() returns, and revoking synchronously saves an empty file.
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }
  return "downloaded";
}
