import type { Session } from "./api";
import { estimateOneRepMax } from "./oneRepMax";

/**
 * As much of a recap as can be worked out from the session alone.
 *
 * The fallback for a finished workout the server has not answered about — the
 * basement case, where the Finish is queued and every GET is a transport
 * failure (see recapHandoff). Everything here comes off the `Session` object
 * the lifter already has: the sets carry reps and weights, and `previousBests`
 * carries each lift's record to beat, which is on the session precisely so the
 * screen can flag a record without asking.
 *
 * It is a DIFFERENT type from SessionRecap rather than a SessionRecap with the
 * unknown fields nulled, and that is the point of it. Nulling them would make
 * `weightDeltaPct === null` mean two things — "there was no last time" and "we
 * could not ask" — and the surface would have no way to tell which message to
 * show. A separate type forces the route to decide once, where the decision is
 * visible.
 *
 * What is missing, and why: pace, the vs-last-time deltas, milestones and the
 * streak all need sessions other than this one, and the estimated-max records
 * need each lift's best e1RM rather than its best weight. None of that is on
 * the wire here. Those sections are absent offline rather than guessed at.
 */

/** How long a session may run before its elapsed time stops meaning anything. */
const MAX_DURATION_SECONDS = 12 * 60 * 60;

export type LocalLift = {
  exerciseId: number;
  exerciseName: string;
  kind: "main" | "assistance";
  topWeightLb: number;
  topReps: number;
  topE1rmLb: number;
  setsLogged: number;
  setsPrescribed: number;
  repsLogged: number;
  repsTargeted: number;
  volumeLb: number;
  hitEveryTarget: boolean;
};

/** A weight record, the only kind derivable from previousBests. */
export type LocalPR = {
  exerciseId: number;
  exerciseName: string;
  weightLb: number;
  previousLb: number;
};

export type LocalRecap = {
  durationSeconds: number | null;
  volumeLb: number;
  setsLogged: number;
  setsPrescribed: number;
  repsLogged: number;
  repsTargeted: number;
  lifts: LocalLift[];
  prs: LocalPR[];
};

export function localRecap(session: Session): LocalRecap {
  const out: LocalRecap = {
    durationSeconds: localDuration(session),
    volumeLb: 0,
    setsLogged: 0,
    setsPrescribed: 0,
    repsLogged: 0,
    repsTargeted: 0,
    lifts: [],
    prs: [],
  };

  // Prescription order, which is the order the server returns sets in — main
  // lifts first, assistance after. A lift with nothing logged gets no row: it
  // was not performed, and a row of zeroes would claim otherwise.
  const byId = new Map<number, LocalLift>();
  for (const set of session.sets) {
    const reps = set.actualReps ?? 0;

    out.setsPrescribed += 1;
    out.repsTargeted += set.targetReps;
    if (reps > 0) {
      out.setsLogged += 1;
      out.repsLogged += reps;
      out.volumeLb += reps * set.weightLb;
    }

    let lift = byId.get(set.exerciseId);
    if (!lift) {
      lift = {
        exerciseId: set.exerciseId,
        exerciseName: set.exerciseName,
        kind: set.kind,
        topWeightLb: 0,
        topReps: 0,
        topE1rmLb: 0,
        setsLogged: 0,
        setsPrescribed: 0,
        repsLogged: 0,
        repsTargeted: 0,
        volumeLb: 0,
        hitEveryTarget: true,
      };
      byId.set(set.exerciseId, lift);
    }

    lift.setsPrescribed += 1;
    lift.repsTargeted += set.targetReps;
    if (!set.completed) lift.hitEveryTarget = false;
    if (reps <= 0) continue;

    lift.setsLogged += 1;
    lift.repsLogged += reps;
    lift.volumeLb += reps * set.weightLb;
    if (set.weightLb > lift.topWeightLb) {
      lift.topWeightLb = set.weightLb;
      lift.topReps = reps;
    }
    // Not always the top set: five reps at 225 estimates above one at 245.
    const e1rm = estimateOneRepMax(set.weightLb, reps);
    if (e1rm > lift.topE1rmLb) lift.topE1rmLb = e1rm;
  }

  out.lifts = [...byId.values()].filter((l) => l.setsLogged > 0);

  const best = new Map(session.previousBests.map((b) => [b.exerciseId, b.weightLb]));
  for (const lift of out.lifts) {
    const previousLb = best.get(lift.exerciseId) ?? 0;
    // A lift with no history is absent from previousBests rather than zero, so
    // `?? 0` reads as "nothing to beat" — the same rule the session screen uses
    // to decide whether a set earns confetti.
    if (lift.topWeightLb > previousLb) {
      out.prs.push({
        exerciseId: lift.exerciseId,
        exerciseName: lift.exerciseName,
        weightLb: lift.topWeightLb,
        previousLb,
      });
    }
  }
  return out;
}

/**
 * Elapsed time from the session's creation to its finish, or null when that
 * cannot be said. Mirrors SessionDuration in internal/racked, including the
 * twelve-hour cap: a session left open overnight measures the tab, not the
 * training, and the two surfaces must not disagree about which sessions have a
 * length once the server's answer arrives and replaces this one.
 */
function localDuration(session: Session): number | null {
  if (!session.finishedAt) return null;
  const started = Date.parse(session.createdAt);
  const finished = Date.parse(session.finishedAt);
  if (!Number.isFinite(started) || !Number.isFinite(finished)) return null;

  const seconds = Math.round((finished - started) / 1000);
  if (seconds <= 0 || seconds > MAX_DURATION_SECONDS) return null;
  return seconds;
}
