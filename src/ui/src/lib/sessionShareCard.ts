/**
 * One workout's share card.
 *
 * The sibling of shareCardContent in shareCard.ts: same ShareCardContent, same
 * layout, same painter — only the selection differs, because a month and a
 * single session are different questions. Nothing below this file knows which
 * of the two it is drawing.
 */

import type { SessionRecap } from "./api";
import { formatLongDate } from "./date";
import { barFraction, formatDelta, formatSessionLength } from "./racked";
import { formatVolume } from "./volume";
import { formatOrdinal, formatPRGain, formatPace } from "./recap";
import { LIFT_ROWS, type MomentRow, type ShareCardContent } from "./shareCard";

/** Rows the records may take, each carrying one lift and what it lifted. */
const PR_ROWS = 4;

/**
 * Moment rows the session card allows, against MOMENT_ROWS' three on the
 * monthly one.
 *
 * A month's card summarises the records as a count and spends its rows on
 * variety — most improved, heaviest set, a milestone. A session has room to be
 * specific, and the records ARE the session: giving each one its own row is the
 * difference between "you set three records" and being shown what they were.
 *
 * Six is not a free choice. The layout packs blocks into the space above the
 * footer, and a card that outgrows it runs long rather than cropping — so this
 * is bounded by what the fullest realistic session card can hold: a header with
 * both optional lines, four stat tiles, five lift bars, and this. See the
 * layout test, which asserts the last block still clears contentBottom.
 */
const SESSION_MOMENT_ROWS = 6;

/**
 * The records, one row each: "PR · Squat" against "245 lb × 5 · +2%".
 *
 * A row apiece rather than one row listing names, because the weight is the
 * thing — "Squat, Bench Press and Barbell Row" says a good day happened without
 * saying what it was. This block used to be a single row reading "Personal
 * records (3)" beside one lift's name, which looked like a card that had failed
 * to render the other two.
 *
 * The two kinds are labelled apart, as they are on the page. A weight record
 * shows the bar and the reps that carried it. An estimated-max record shows the
 * estimate, because the bar did not move and quoting it would make the row look
 * like a record that isn't one.
 *
 * Sorted heaviest first, so the lift that leads is worth leading with and the
 * ones that overflow are the least impressive — the server returns records
 * alphabetically, which is right for a list and arbitrary for a headline.
 */
function prMoments(prs: SessionRecap["prs"]): MomentRow[] {
  const ranked = [...prs].sort((a, b) => b.valueLb - a.valueLb);
  // Only give up a row to the overflow count when there is something to count.
  const shown = ranked.length > PR_ROWS ? ranked.slice(0, PR_ROWS - 1) : ranked;

  const rows: MomentRow[] = shown.map((pr) => {
    const lifted =
      pr.kind === "weight"
        ? `${formatVolume(pr.weightLb)} lb × ${pr.reps}`
        : `${formatVolume(pr.valueLb)} lb`;
    // How far it moved the mark, when that can be said at all — a lift with no
    // history has nothing to be a percentage of. See formatPRGain.
    const gain = formatPRGain(pr.valueLb, pr.previousLb);
    return {
      label: `${pr.kind === "weight" ? "PR" : "Est. max"} · ${pr.exerciseName}`,
      value: gain ? `${lifted} · ${gain}` : lifted,
    };
  });

  const rest = ranked.length - shown.length;
  if (rest > 0) rows.push({ label: "More records", value: `+${rest}` });
  return rows;
}

export function sessionShareCardContent(
  recap: SessionRecap,
  displayName: string = "",
): ShareCardContent {
  const name = displayName.trim();

  const comparison =
    recap.volume.comparison.count > 0
      ? `That's ${recap.volume.comparison.count.toLocaleString("en-US")} ${recap.volume.comparison.label}.`
      : null;

  // The paired-lift progression, which is the line this whole feature is
  // about — but only when it was drawn from something. liftsCompared is 0 on a
  // first workout and when nothing lined up, and "+0%" there would be a claim
  // rather than an absence.
  const pct = recap.progress.weightDeltaPct;
  const change =
    pct != null && recap.progress.liftsCompared > 0
      ? `${formatDelta(pct)} on your last ${recap.session.programDayName}`
      : null;

  // Heaviest first, so the card leads with the lift the workout was about.
  // The server returns them in prescription order, which is right for reading
  // a recap against the workout and wrong for a card with room for five.
  const top = [...recap.lifts].sort((a, b) => b.volumeLb - a.volumeLb).slice(0, LIFT_ROWS);
  const heaviest = Math.max(0, ...top.map((l) => l.volumeLb));
  const lifts = top.map((l) => ({
    name: l.exerciseName,
    value: `${formatVolume(l.topWeightLb)} lb × ${l.topReps}`,
    fraction: barFraction(l.volumeLb, heaviest),
  }));

  const moments: MomentRow[] = [];
  if (recap.pace) {
    moments.push({
      label: recap.pace.rank === 1 ? "Fastest yet" : "Pace",
      value:
        recap.pace.rank === 1
          ? formatSessionLength(recap.durationSeconds ?? 0)
          : `${formatPace(recap.pace.deltaPct)} · ${formatOrdinal(recap.pace.rank)} of ${recap.pace.of}`,
    });
  }
  moments.push(...prMoments(recap.prs));
  if (recap.milestones.length > 0) {
    moments.push({ label: "Milestone", value: recap.milestones[0].label });
  }

  return {
    // The date formatted rather than raw: this was the one place in the app that
    // put a bare "2026-09-24" in front of a reader. Absolute, and uppercased to
    // match the eyebrow it sits in — a share card is a picture, so there is no
    // hover here and nothing to recover a relative form from.
    eyebrow: [
      recap.session.programDayName.toUpperCase(),
      formatLongDate(recap.session.performedOn).toUpperCase(),
    ].join(" · "),
    lede: name ? `${name} lifted` : "You lifted",
    headline: `${formatVolume(recap.volume.totalLb)} LB`,
    comparison,
    change,
    // No archetype: that is a judgement about how a month of training went, and
    // one session is not a month. The layout gives an absent panel no space.
    archetype: null,
    tiles: [
      {
        value: recap.durationSeconds ? formatSessionLength(recap.durationSeconds) : "—",
        label: "duration",
      },
      { value: `${recap.volume.setsLogged}`, label: "sets" },
      { value: `${recap.volume.repsLogged}`, label: "reps" },
      { value: `${recap.streak.sessions}`, label: "session streak" },
    ],
    lifts,
    moments: moments.slice(0, SESSION_MOMENT_ROWS),
    footnote: `${recap.session.programName} · ${recap.session.programDayName}`,
  };
}
