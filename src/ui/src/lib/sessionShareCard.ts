/**
 * One workout's share card.
 *
 * The sibling of shareCardContent in shareCard.ts: same ShareCardContent, same
 * layout, same painter — only the selection differs, because a month and a
 * single session are different questions. Nothing below this file knows which
 * of the two it is drawing.
 */

import type { SessionRecap } from "./api";
import { formatDelta } from "./racked";
import { formatVolume } from "./volume";
import { formatOrdinal, formatPace } from "./recap";
import { barFraction } from "./racked";
import { LIFT_ROWS, MOMENT_ROWS, type MomentRow, type ShareCardContent } from "./shareCard";
import { formatSessionLength } from "./racked";

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
    const best = recap.prs[0];
    moments.push({
      label: recap.prs.length === 1 ? "Personal record" : `Personal records (${recap.prs.length})`,
      value: `${best.exerciseName} ${formatVolume(best.valueLb)} lb`,
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
