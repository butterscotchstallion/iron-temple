// Avatar helpers, kept out of the component so they can be unit-tested without
// rendering.

// Palette for derived avatar colours — the synthwave theme's accents (see the
// @theme block in app.css), so a generated chip looks like it belongs.
const PALETTE = [
  "#b026ff", // neon
  "#ff2fb9", // magenta
  "#05d9e8", // cyan
  "#ff6ac1", // sun
  "#7b2ff7", // neon-soft
];

/**
 * The colour for a user's initials chip: their chosen one, or a stable pick
 * from the palette keyed on their id. Derived rather than random so the same
 * person is the same colour on every render and every device.
 */
export function avatarColor(id: number, chosen: string): string {
  if (chosen) return chosen;
  // ids are small positive integers, so a modulo is a good enough spread.
  return PALETTE[Math.abs(Math.trunc(id)) % PALETTE.length];
}

/**
 * Up to two initials for a display name. Falls back to the first character of
 * whatever it was given, and to "?" for an empty name — a chip with no letters
 * reads as a broken image.
 */
export function initials(displayName: string): string {
  const words = displayName.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "?";
  if (words.length === 1) {
    // Intl-aware: [...str] splits by code point, so an emoji or a non-BMP
    // character yields one initial rather than half a surrogate pair.
    return [...words[0]].slice(0, 1).join("").toUpperCase();
  }
  const first = [...words[0]][0] ?? "";
  const last = [...words[words.length - 1]][0] ?? "";
  return (first + last).toUpperCase();
}

/**
 * The URL for a user's avatar image. The etag is appended as a cache-buster so
 * a fresh upload appears immediately instead of after the cache revalidates.
 */
export function avatarUrl(id: number, etag: string | undefined): string {
  const base = `/api/v1/users/${id}/avatar`;
  return etag ? `${base}?v=${encodeURIComponent(etag)}` : base;
}
