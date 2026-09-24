/**
 * One crown's share card.
 *
 * The third selector feeding shareCard.ts's layout and painter, after the monthly
 * recap and the single session. Same `ShareCardContent`, same blocks, same pixels
 * — only the selection differs, because "what I hold" is a different question from
 * "what I moved".
 *
 * WHAT MAKES THIS CARD DIFFERENT FROM THE OTHER TWO
 *
 * They are about a quantity, so their headline is a number and their body is where
 * the weight went. This one is about a standing, so the headline is the name of the
 * thing and the body has to say why it was hard — which is what the catalogue's
 * description is for, and why it goes in the `comparison` slot rather than being
 * dropped. A card reading "TOP OF WEEK STREAK" over nothing is a brag nobody
 * outside the install can read.
 *
 * It also carries no lift bars. There is no breakdown of a crown; the reign facts
 * go in the tiles instead, which is the one block whose meaning is "two numbers
 * worth knowing".
 */

import type { LifterAchievement, RackedUpcomingMilestone } from "./api";
import { formatLongDate } from "./date";
import { formatVolume } from "./volume";
import type { MomentRow, ShareCardContent } from "./shareCard";

/**
 * "20 lb to go" — how far short of a rung the lifter is.
 *
 * Rounded to the pound like every other weight in the app. A remainder below a
 * pound is "0 lb to go" on a target not yet reached, which reads as a bug, so it
 * floors at 1.
 */
export function formatRemaining(upcoming: RackedUpcomingMilestone): string {
  const remaining = Math.max(1, Math.round(upcoming.targetLb - upcoming.currentLb));
  return `${formatVolume(remaining)} lb to go`;
}

export function achievementShareCardContent(
  held: LifterAchievement,
  displayName: string = "",
  // The nearest thing they are chasing, when there is one. Optional because a
  // lifter past every rung has nothing to add here, and the row is simply absent
  // rather than a line reading "nothing".
  nearest: RackedUpcomingMilestone | null = null,
): ShareCardContent {
  const name = displayName.trim();

  const moments: MomentRow[] = [];
  if (nearest) {
    moments.push({ label: "Closing in", value: `${nearest.label} · ${formatRemaining(nearest)}` });
  }

  return {
    eyebrow: "IRON TEMPLE · CROWN",
    // Third person when there is a name and second when there is not, matching
    // the other two cards exactly — the sender is showing this to other people.
    lede: name ? `${name} holds` : "You hold",
    headline: held.achievement.label.toUpperCase(),
    // The description, which on this card is the whole argument for why the
    // headline is worth anything. Never null: the catalogue always carries one.
    comparison: held.achievement.description,
    // Reserved on the other cards for a period-over-period delta, and there is no
    // such thing for a standing.
    change: null,
    // A judgement about how a month of training went, which a crown is not.
    archetype: null,
    tiles: [
      { value: String(held.timesHeld), label: held.timesHeld === 1 ? "time held" : "times held" },
      {
        value: formatLongDate(held.lastHeldFrom),
        // Present tense only when it is still theirs. A card saying "holding
        // since" about a crown somebody lost in August is the one way this could
        // be dishonest.
        label: held.heldNow ? "holding since" : "last held",
      },
    ],
    lifts: [],
    moments,
    footnote: held.heldNow
      ? "Top of the board on this install."
      : "Held the top of this board.",
  };
}

/** `crown-top-of-week-streak.png`, from the achievement's own slug. */
export function achievementShareCardFilename(held: LifterAchievement): string {
  return `${held.achievement.slug || "crown"}.png`;
}
