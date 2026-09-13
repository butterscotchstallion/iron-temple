/**
 * One workout's share card.
 *
 * The sibling of shareCardContent in shareCard.ts: same ShareCardContent, same
 * layout, same painter — only the selection differs, because a month and a
 * single session are different questions. Nothing below this file knows which
 * of the two it is drawing.
 */

import type { SessionRecap } from "./api";
import { barFraction, formatDelta, formatSessionLength, joinNames } from "./racked";
import { formatVolume } from "./volume";
import { formatOrdinal, formatPace } from "./recap";
import { LIFT_ROWS, MOMENT_ROWS, type MomentRow, type ShareCardContent } from "./shareCard";

/**
 * The most comma-separated parts the records row will carry.
 *
 * Three either way: three names when that is all there are, or two names and a
 * count when there are more. The row is painted right-aligned into 560px at
 * 26px semibold, and fitText clips what does not fit — "Squat, Bench Press,
 * Barbell Row and 2 more" already sits on that limit, and lifts are not always
 * called "Squat". Two names plus a count leaves room for the ones that aren't.
 */
const PR_PARTS = 3;

/**
 * The records, as one line: "Squat 245 lb", or "Squat, Bench Press and Row".
 *
 * A single record has room for its weight, which is the interesting part when
 * there is only one. Beyond that the names are what the lifter wants to see, so
 * they are joined the way a person says them — the same joinNames the recap
 * email and the Racked page use, rather than a third spelling of the same list.
 *
 * The row never states a count it does not then show. It used to read
 * "Personal records (3)" against a single lift's name, which looks like a card
 * that failed to render the other two. Where there are more records than the
 * line can carry, the overflow is named as an overflow ("and 2 more") — which
 * is a promise the line keeps.
 *
 * Sorted heaviest first, so the lift that leads is the one worth leading with
 * and the ones dropped are the least impressive. The server returns records in
 * alphabetical order — fine for a list, arbitrary for a headline, and the
 * reason this used to put whichever lift sorted first beside a count of three.
 */
function prsValue(prs: SessionRecap["prs"]): string {
  const ranked = [...prs].sort((a, b) => b.valueLb - a.valueLb);
  if (ranked.length === 1) {
    return `${ranked[0].exerciseName} ${formatVolume(ranked[0].valueLb)} lb`;
  }
  if (ranked.length <= PR_PARTS) {
    return joinNames(ranked.map((p) => p.exerciseName));
  }
  const named = ranked.slice(0, PR_PARTS - 1).map((p) => p.exerciseName);
  return joinNames([...named, `${ranked.length - named.length} more`]);
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
  if (recap.prs.length > 0) {
    moments.push({
      label: recap.prs.length === 1 ? "Personal record" : "Personal records",
      value: prsValue(recap.prs),
    });
  }
  if (recap.milestones.length > 0) {
    moments.push({ label: "Milestone", value: recap.milestones[0].label });
  }

  return {
    eyebrow: `${recap.session.programDayName.toUpperCase()} · ${recap.session.performedOn}`,
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
    moments: moments.slice(0, MOMENT_ROWS),
    footnote: `${recap.session.programName} · ${recap.session.programDayName}`,
  };
}
