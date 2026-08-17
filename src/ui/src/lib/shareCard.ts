/**
 * The Racked share card — the recap reduced to one 1080×1350 image.
 *
 * Three pure steps and one impure one, in that order:
 *
 *   shareCardContent()  report        -> the strings and fractions that appear
 *   shareCardLayout()   content       -> where each block sits, vertically
 *   paintShareCard()    ctx + content -> pixels
 *
 * The split is deliberate. A canvas cannot be inspected from a unit test — jsdom
 * has no 2D context at all — so anything left inside the painter is untestable
 * by construction. Keeping selection and geometry outside it means the two
 * things that actually go wrong (a section read off a null field, a card whose
 * content runs past its own footer) are caught by assertions on plain objects
 * rather than by looking at a picture.
 *
 * Nothing here computes a statistic. Every figure is already in the report the
 * page fetched, and the recap email renders the same ones from the same source
 * (internal/racked/template.go), so the card cannot disagree with either.
 */

import type { RackedReport } from "./api";
import { formatVolume } from "./volume";
import { formatDelta, formatPercent, formatPerWeek, barFraction } from "./racked";

/**
 * Size and palette.
 *
 * 1080×1350 is the 4:5 portrait every feed accepts uncropped, and the tallest
 * shape that is not a story — the card has a lot to say and 1:1 would cost it a
 * section.
 *
 * The palette is fixed rather than read from the theme. A share card is looked
 * at by people who do not have the app, so it should look the same for every
 * sender; and the app's colours are OKLCH custom properties that would need
 * resolving against a live element at draw time and would produce a washed-out
 * white card for anyone running the light theme. These are the recap email's
 * header colours (#2a1250) and the app's neon accent, which is the closest
 * thing Iron Temple has to a brand.
 */
export const SHARE_CARD = {
  width: 1080,
  height: 1350,
  pad: 76,

  bgTop: "#2a1250",
  bgBottom: "#150a29",
  neon: "#b026ff",
  neonSoft: "#7b2ff7",
  white: "#ffffff",
  text: "#eae0ff",
  muted: "#a08cc0",
  rule: "rgba(176, 38, 255, 0.28)",
  panel: "rgba(176, 38, 255, 0.12)",
  track: "rgba(234, 224, 255, 0.10)",
} as const;

/** How many lifts the "where the weight went" block carries. Matches the email. */
export const LIFT_ROWS = 5;
/** How many moment rows fit under the lifts. */
export const MOMENT_ROWS = 3;

const FONT_STACK = '"Inter Variable", ui-sans-serif, system-ui, sans-serif';

/** The canvas font shorthand for a weight and size in the app's typeface. */
export function font(weight: number, size: number): string {
  return `${weight} ${size}px ${FONT_STACK}`;
}

export type LiftBar = { name: string; value: string; fraction: number };
export type MomentRow = { label: string; value: string };

export type ShareCardContent = {
  eyebrow: string;
  /** "Ada lifted", or "You lifted" when there is no display name to use. */
  lede: string;
  headline: string;
  /** "That's 3 school buses." — absent when the volume is too small to compare. */
  comparison: string | null;
  /** "+12% on the previous month" — absent when there is no period to compare to. */
  change: string | null;
  archetype: { name: string; description: string } | null;
  tiles: { value: string; label: string }[];
  lifts: LiftBar[];
  moments: MomentRow[];
  footnote: string;
};

/**
 * Choose what goes on the card.
 *
 * Every optional section comes back null or empty when the period has no answer
 * for it, and the layout below then gives it no space at all — a lifter's first
 * month has no previous period to compare against and no lift performed twice,
 * and a card with two empty panels in it reads as a broken card rather than a
 * quiet month.
 */
export function shareCardContent(
  report: RackedReport,
  displayName: string = "",
): ShareCardContent {
  const name = displayName.trim();

  const comparison =
    report.comparison.count > 0
      ? `That's ${report.comparison.count.toLocaleString("en-US")} ${report.comparison.label}.`
      : null;

  // The API sends volumePct as null when the previous period moved no weight,
  // because growth from zero is not a ratio — so this asks for the field, not
  // just for the change object.
  const pct = report.change?.volumePct;
  const change =
    pct != null ? `${formatDelta(pct)} on the previous ${report.period.kind}` : null;

  // Heaviest first, server-side (sortLifts, internal/racked/racked.go), so the
  // top five here are the five the lifter spent the period on — and the same
  // five the email shows.
  const top = report.lifts.slice(0, LIFT_ROWS);
  const heaviest = Math.max(0, ...top.map((l) => l.volumeLb));
  const lifts = top.map((l) => ({
    name: l.exerciseName,
    value: `${formatVolume(l.volumeLb)} lb`,
    fraction: barFraction(l.volumeLb, heaviest),
  }));

  const moments: MomentRow[] = [];
  if (report.mostImproved) {
    moments.push({
      label: "Most improved",
      value: `${report.mostImproved.exerciseName} ${formatDelta(report.mostImproved.gainPct)}`,
    });
  }
  if (report.heaviestSet) {
    moments.push({
      label: "Heaviest set",
      value: `${formatVolume(report.heaviestSet.weightLb)} lb × ${report.heaviestSet.reps}`,
    });
  }
  if (report.prs.length > 0) {
    moments.push({
      label: report.prs.length === 1 ? "Personal record" : "Personal records",
      value: `${report.prs.length}`,
    });
  }

  // The same choice the page makes: a rate only where a rate exists, and a
  // measured frequency where the program carries no schedule to grade against.
  const footnote =
    report.attendance.basis === "weekday"
      ? `${formatPercent(report.attendance.rate)} of scheduled sessions`
      : `${formatPerWeek(report.attendance.sessionsPerWeek)} sessions a week`;

  return {
    eyebrow: `RACKED · ${report.period.label.toUpperCase()}`,
    lede: name ? `${name} lifted` : "You lifted",
    headline: `${formatVolume(report.totals.volumeLb)} LB`,
    comparison,
    change,
    archetype: report.archetype.name
      ? { name: report.archetype.name.toUpperCase(), description: report.archetype.description }
      : null,
    tiles: [
      { value: `${report.totals.sessions}`, label: "sessions" },
      { value: `${report.totals.sets}`, label: "sets" },
      { value: `${report.totals.reps}`, label: "reps" },
      { value: `${report.streak.longestWeeks}`, label: "week streak" },
    ],
    lifts,
    moments: moments.slice(0, MOMENT_ROWS),
    footnote,
  };
}

// Vertical metrics. Every one of these is an advance, not a font size: the gap
// is baked in, so a block's height is the sum of the lines it draws and the
// layout never has to reason about leading.
const M = {
  eyebrow: 44,
  lede: 44,
  headline: 108,
  comparison: 44,
  change: 38,

  gap: 32,
  archetype: 126,
  tiles: 116,
  sectionTitle: 46,
  liftRow: 50,
  momentRow: 44,

  footer: 96,
} as const;

export type BlockKind = "header" | "archetype" | "tiles" | "lifts" | "moments";
export type Block = { kind: BlockKind; y: number; height: number };
export type ShareCardLayout = {
  blocks: Block[];
  /** Top of the footer rule. Pinned to the bottom, never packed with the rest. */
  footerY: number;
  /**
   * The lowest pixel a block may occupy. One gap clear of the footer rule, so
   * the fullest card still reads as having a footer rather than a line drawn
   * through the end of its last section.
   */
  contentBottom: number;
};

/**
 * Place each block down the card.
 *
 * Blocks are measured, stacked, and then the whole stack is centred between the
 * top pad and `contentBottom`. Centring rather than top-aligning is what keeps a
 * sparse card — one session, no archetype, no records — from sitting in the top
 * third with a third of the image blank underneath it.
 *
 * The fullest report the API can produce has to fit that region without being
 * clamped, and shareCard.test.ts asserts exactly that. It is the one property of
 * this card no one can check by looking at the running app: an overflow only
 * shows up on the specific month that happens to fill every section.
 */
export function shareCardLayout(content: ShareCardContent): ShareCardLayout {
  const heights: Block[] = [];

  let header = M.eyebrow + M.lede + M.headline;
  if (content.comparison) header += M.comparison;
  if (content.change) header += M.change;
  heights.push({ kind: "header", y: 0, height: header });

  if (content.archetype) heights.push({ kind: "archetype", y: 0, height: M.archetype });
  heights.push({ kind: "tiles", y: 0, height: M.tiles });
  if (content.lifts.length > 0) {
    heights.push({
      kind: "lifts",
      y: 0,
      height: M.sectionTitle + content.lifts.length * M.liftRow,
    });
  }
  if (content.moments.length > 0) {
    heights.push({
      kind: "moments",
      y: 0,
      height: M.sectionTitle + content.moments.length * M.momentRow,
    });
  }

  const footerY = SHARE_CARD.height - M.footer;
  const contentBottom = footerY - M.gap;
  const available = contentBottom - SHARE_CARD.pad;
  const total =
    heights.reduce((sum, b) => sum + b.height, 0) + M.gap * (heights.length - 1);

  // Never above the top pad, however tall the content — a card that somehow
  // outgrew its region starts at the pad and runs long rather than starting
  // off-image where the first line would be cropped away entirely.
  let y = Math.max(SHARE_CARD.pad, SHARE_CARD.pad + (available - total) / 2);
  const blocks = heights.map((b) => {
    const placed = { ...b, y };
    y += b.height + M.gap;
    return placed;
  });

  return { blocks, footerY, contentBottom };
}

/**
 * The subset of a 2D context the painter touches.
 *
 * Named so a test can hand in a recording stub: jsdom returns null from
 * getContext("2d"), and the alternative — shipping a native canvas binding as a
 * dev dependency purely to assert that the painter does not throw — is a large
 * cost for a small assertion.
 */
export type Painter = Pick<
  CanvasRenderingContext2D,
  | "fillRect"
  | "fillText"
  | "measureText"
  | "createLinearGradient"
  | "save"
  | "restore"
  | "beginPath"
  | "fill"
> & {
  fillStyle: string | CanvasGradient | CanvasPattern;
  font: string;
  textAlign: CanvasTextAlign;
  textBaseline: CanvasTextBaseline;
  letterSpacing?: string;
  roundRect?: CanvasRenderingContext2D["roundRect"];
};

/**
 * Shorten text to fit a width, with an ellipsis. Trims one character at a time
 * from a string that is already close to the limit — exercise names are short
 * enough that a binary search would cost more to read than it saves to run.
 */
export function fitText(ctx: Painter, text: string, maxWidth: number): string {
  if (ctx.measureText(text).width <= maxWidth) return text;
  let cut = text;
  while (cut.length > 1 && ctx.measureText(`${cut}…`).width > maxWidth) {
    cut = cut.slice(0, -1);
  }
  return `${cut.trimEnd()}…`;
}

/** A rounded rectangle, falling back to square corners where roundRect is absent. */
function fillRoundRect(
  ctx: Painter,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
): void {
  if (w <= 0 || h <= 0) return;
  if (typeof ctx.roundRect !== "function") {
    ctx.fillRect(x, y, w, h);
    return;
  }
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, Math.min(r, w / 2, h / 2));
  ctx.fill();
}

/** Draw a line of text, optionally letter-spaced, and return nothing. */
function line(
  ctx: Painter,
  text: string,
  x: number,
  y: number,
  opts: {
    font: string;
    fill: string;
    align?: CanvasTextAlign;
    spacing?: string;
    /** Truncate to this width. Measured in this line's own font, not the last one's. */
    max?: number;
  },
): void {
  ctx.font = opts.font;
  ctx.fillStyle = opts.fill;
  ctx.textAlign = opts.align ?? "left";
  ctx.letterSpacing = opts.spacing ?? "0px";
  // Truncation happens here, after the font and letter spacing are set, and
  // never at the call site. Passing fitText(ctx, …) as an argument would run it
  // before this function set the font, so every string would be measured in
  // whichever font the previous line happened to leave behind — the comparison
  // line measured at the headline's 96px and drawn at 30px, and so on down the
  // card. Keeping the two together makes that ordering impossible to get wrong.
  ctx.fillText(opts.max ? fitText(ctx, text, opts.max) : text, x, y);
  ctx.letterSpacing = "0px";
}

/** Paint the whole card. The canvas must already be SHARE_CARD.width × .height. */
export function paintShareCard(ctx: Painter, content: ShareCardContent): void {
  const { width: W, height: H, pad } = SHARE_CARD;
  const right = W - pad;
  const inner = W - pad * 2;
  const layout = shareCardLayout(content);

  ctx.textBaseline = "top";

  const bg = ctx.createLinearGradient(0, 0, 0, H);
  bg.addColorStop(0, SHARE_CARD.bgTop);
  bg.addColorStop(1, SHARE_CARD.bgBottom);
  ctx.fillStyle = bg;
  ctx.fillRect(0, 0, W, H);

  for (const block of layout.blocks) {
    switch (block.kind) {
      case "header":
        paintHeader(ctx, content, block.y);
        break;
      case "archetype":
        paintArchetype(ctx, content, block.y);
        break;
      case "tiles":
        paintTiles(ctx, content, block.y);
        break;
      case "lifts":
        paintLifts(ctx, content, block.y);
        break;
      case "moments":
        paintMoments(ctx, content, block.y);
        break;
    }
  }

  // Footer: a rule, the wordmark, and the one figure that did not earn a block.
  ctx.fillStyle = SHARE_CARD.rule;
  ctx.fillRect(pad, layout.footerY, inner, 2);
  line(ctx, "IRON TEMPLE", pad, layout.footerY + 30, {
    font: font(800, 26),
    fill: SHARE_CARD.neon,
    spacing: "5px",
  });
  line(ctx, content.footnote, right, layout.footerY + 32, {
    font: font(400, 24),
    fill: SHARE_CARD.muted,
    align: "right",
  });
}

function paintHeader(ctx: Painter, content: ShareCardContent, top: number): void {
  const { pad } = SHARE_CARD;
  let y = top;

  line(ctx, content.eyebrow, pad, y, {
    font: font(700, 24),
    fill: SHARE_CARD.neon,
    spacing: "5px",
  });
  y += M.eyebrow;

  line(ctx, content.lede, pad, y, {
    font: font(400, 30),
    fill: SHARE_CARD.muted,
    max: SHARE_CARD.width - pad * 2,
  });
  y += M.lede;

  line(ctx, content.headline, pad, y, { font: font(800, 96), fill: SHARE_CARD.white });
  y += M.headline;

  if (content.comparison) {
    line(ctx, content.comparison, pad, y, {
      font: font(400, 30),
      fill: SHARE_CARD.text,
      max: SHARE_CARD.width - pad * 2,
    });
    y += M.comparison;
  }
  if (content.change) {
    line(ctx, content.change, pad, y, { font: font(400, 26), fill: SHARE_CARD.muted });
  }
}

function paintArchetype(ctx: Painter, content: ShareCardContent, top: number): void {
  if (!content.archetype) return;
  const { pad } = SHARE_CARD;
  const inner = SHARE_CARD.width - pad * 2;

  ctx.fillStyle = SHARE_CARD.panel;
  fillRoundRect(ctx, pad, top, inner, M.archetype - 8, 20);

  // Placed against the panel's own height (M.archetype - 8 = 118): the title
  // ends at 42, the name at 84, the description at 114 — four pixels inside the
  // rounded edge rather than flush against it.
  line(ctx, "YOUR LIFTER TYPE", pad + 32, top + 22, {
    font: font(700, 20),
    fill: SHARE_CARD.muted,
    spacing: "4px",
  });
  line(ctx, content.archetype.name, pad + 32, top + 50, {
    font: font(800, 34),
    fill: SHARE_CARD.white,
    max: inner - 64,
  });
  line(ctx, content.archetype.description, pad + 32, top + 92, {
    font: font(400, 22),
    fill: SHARE_CARD.text,
    max: inner - 64,
  });
}

function paintTiles(ctx: Painter, content: ShareCardContent, top: number): void {
  const { pad } = SHARE_CARD;
  const inner = SHARE_CARD.width - pad * 2;
  const column = inner / content.tiles.length;

  content.tiles.forEach((tile, i) => {
    const centre = pad + column * i + column / 2;
    line(ctx, tile.value, centre, top + 8, {
      font: font(800, 52),
      fill: SHARE_CARD.white,
      align: "center",
    });
    line(ctx, tile.label.toUpperCase(), centre, top + 74, {
      font: font(600, 20),
      fill: SHARE_CARD.muted,
      align: "center",
      spacing: "3px",
    });
  });
}

function paintLifts(ctx: Painter, content: ShareCardContent, top: number): void {
  const { pad } = SHARE_CARD;
  const right = SHARE_CARD.width - pad;

  line(ctx, "WHERE THE WEIGHT WENT", pad, top, {
    font: font(700, 20),
    fill: SHARE_CARD.muted,
    spacing: "4px",
  });

  // Three columns: name, bar, value. The bar takes what the other two leave.
  const nameWidth = 260;
  const valueWidth = 190;
  const barX = pad + nameWidth + 24;
  const barWidth = right - valueWidth - 24 - barX;

  content.lifts.forEach((lift, i) => {
    const y = top + M.sectionTitle + i * M.liftRow;

    line(ctx, lift.name, pad, y + 6, {
      font: font(600, 26),
      fill: SHARE_CARD.white,
      max: nameWidth,
    });

    ctx.fillStyle = SHARE_CARD.track;
    fillRoundRect(ctx, barX, y + 12, barWidth, 16, 8);
    // The first bar is the period's heaviest lift and gets the full accent; the
    // rest step back so the ranking reads at a glance rather than needing the
    // numbers on the right.
    ctx.fillStyle = i === 0 ? SHARE_CARD.neon : SHARE_CARD.neonSoft;
    fillRoundRect(ctx, barX, y + 12, barWidth * lift.fraction, 16, 8);

    line(ctx, lift.value, right, y + 8, {
      font: font(400, 24),
      fill: SHARE_CARD.text,
      align: "right",
    });
  });
}

function paintMoments(ctx: Painter, content: ShareCardContent, top: number): void {
  const { pad } = SHARE_CARD;
  const right = SHARE_CARD.width - pad;

  line(ctx, "MOMENTS", pad, top, {
    font: font(700, 20),
    fill: SHARE_CARD.muted,
    spacing: "4px",
  });

  content.moments.forEach((moment, i) => {
    const y = top + M.sectionTitle + i * M.momentRow;
    line(ctx, moment.label, pad, y, { font: font(400, 26), fill: SHARE_CARD.muted });
    line(ctx, moment.value, right, y, {
      font: font(600, 26),
      fill: SHARE_CARD.white,
      align: "right",
      max: 560,
    });
  });
}
