/**
 * One milestone's share card.
 *
 * The fourth selector feeding shareCard.ts's layout and painter, and it exists to
 * draw the scarcity line. A crown and a milestone are the two things in this app
 * worth showing somebody; a personal record is not, and deliberately gets no card.
 * That is the whole point of a tier: if everything can be posted, posting means
 * nothing.
 *
 * Built on achievementShareCard's shape rather than the recap's, because the
 * question is the same shape as a crown's — "what I reached", not "what I moved".
 * So the headline is the name of the thing, there are no lift bars, and the body
 * has to say why it was hard.
 *
 * WHAT IT SAYS INSTEAD OF A CATALOGUE DESCRIPTION
 *
 * A crown has one: the catalogue carries prose per board. A milestone has only its
 * label, which is already the headline, so repeating it in `comparison` would be
 * the same sentence twice. What goes there instead is the fact that makes a rung
 * worth having — it can only be passed once per lift, ever.
 */

import type { RackedMilestone, RackedUpcomingMilestone } from "./api";
import { formatRemaining } from "./achievementShareCard";
import { formatLongDate } from "./date";
import { formatVolume } from "./volume";
import type { MomentRow, ShareCardContent } from "./shareCard";

/**
 * A milestone, as a card.
 *
 * `nextUp` is what to chase next, when there is something — the same forward-
 * looking row the crown card carries, and for the same reason: a card that only
 * looks backwards is a trophy shelf, and the thing that makes a milestone land is
 * that there is another one.
 */
export function milestoneShareCardContent(
  milestone: RackedMilestone,
  displayName: string = "",
  nextUp: RackedUpcomingMilestone | null = null,
): ShareCardContent {
  const name = displayName.trim();

  const moments: MomentRow[] = [];
  if (nextUp) {
    moments.push({ label: "Next up", value: `${nextUp.label} · ${formatRemaining(nextUp)}` });
  }

  return {
    eyebrow: "IRON TEMPLE · MILESTONE",
    // Third person with a name, second without — matching all three other cards,
    // because the sender is showing this to other people.
    lede: name ? `${name} reached` : "You reached",
    headline: milestone.label.toUpperCase(),
    // Why a rung is worth anything, since unlike a crown there is no catalogue
    // description to lean on and the label is already the headline.
    comparison:
      milestone.kind === "volume"
        ? "A lifetime total, passed once and never again."
        : "A named weight, and it can only be a first once.",
    // A period-over-period delta, which a threshold does not have.
    change: null,
    archetype: null,
    tiles: [
      {
        // The weight itself, spelled out. The headline carries it too, but the
        // headline is a sentence and this is the number — the same division of
        // labour the other cards use between a headline and a tile.
        value: `${formatVolume(milestone.valueLb)} lb`,
        label: milestone.kind === "volume" ? "lifted, all time" : "on the bar",
      },
      {
        // Date-only, unlike the crown card: a milestone is dated by the SESSION
        // that carried it, which the API sends as a date rather than an instant.
        value: formatLongDate(milestone.performedOn),
        label: "reached on",
      },
    ],
    // There is no breakdown of a single rung, the same call the crown card makes.
    lifts: [],
    moments,
    footnote:
      milestone.kind === "volume"
        ? "Lifetime tonnage, every logged set since day one."
        : `First time at this weight on the ${milestone.exerciseName}.`,
  };
}

/**
 * `first-225-lb-squat.png`, from the label.
 *
 * Slugged from the label rather than composed from the parts, because the label is
 * already the one canonical wording of a rung — the server builds it, the recap
 * email prints it, and the card's headline is it uppercased. Composing a second
 * version here would be a fourth place for "First 225 lb Squat" to be spelled.
 */
export function milestoneShareCardFilename(label: string): string {
  const slug = label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return `${slug || "milestone"}.png`;
}
