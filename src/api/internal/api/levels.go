package api

import (
	"net/http"

	"gitea.homelab/gitadmin/iron-temple/api/internal/levels"
)

// What a lifter's training adds up to, as a number they wear beside their name.
//
// # WHY THIS IS DERIVED AND NOT STORED
//
// This is the opposite answer to the one achievements.go gives, and the difference
// is the cost of the question rather than a difference of taste. A crown means
// "leads a board right now", which takes a Racked report per lifter to decide, so
// it is reconciled on a schedule and written down. A level means "has trained this
// many times", which is a grouped count over an indexed column — cheap enough to
// ask on every poll, and so not worth the two things storing it would cost:
//
//   - A session edited after it is over would leave the stored total behind.
//     Sessions ARE edited after they are over; updateSessionSet amends a finished
//     session on purpose, so this is a live case rather than a hypothetical.
//   - A session deleted would leave experience behind that nothing explains, and
//     no amount of care at the write site fixes a number whose source is gone.
//
// Deriving it also means the award is idempotent for free, which matters more here
// than it looks: finishing a session is queued offline and replayed, so a write-time
// award would have to defend against paying twice for one workout. There is nothing
// to pay twice when there is nothing to pay.
//
// And it means generated lifters have levels without activity.go knowing this
// feature exists. A seeded install's roster is populated because the sessions are
// real, not because the seeder was taught to grant experience.
//
// # WHY IT ANSWERS FOR EVERYBODY AT ONCE
//
// getAchievements' reason exactly: this is the read a client makes in order to draw
// an ornament beside somebody else's name, and names are everywhere — the feed, the
// roster, the leaderboard, comment authors, the header. Answering per lifter would
// be a request per row.
//
// The response is ETagged by the jsonETag middleware on the group, so a poll that
// finds nothing changed costs a 304 and no body. Note that jsonETag hashes the
// body AFTER the handler has run, so a 304 still pays for the query — true of
// /houses and /achievements too, and worth knowing before an install with a
// hundred thousand generated sessions is surprised by it.
//
// The payload can also change with no write at all, because the qualifying rule
// reads now(): an unfinished session ages past the twelve-hour cutoff on its own.
// The ETag is a digest of the bytes and handles that correctly. Nothing here
// should be tempted into invalidating on writes instead.

// getLevels serves every lifter's level.
//
// Every account is listed, including one that has never trained — see
// lifterLevelListDTO for why that is not the payload waste it looks like.
func (s *Server) getLevels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListLifterLevels(r.Context())
	if err != nil {
		internalError(w)
		return
	}

	items := make([]lifterLevelDTO, 0, len(rows))
	for _, row := range rows {
		// The curve lives in internal/levels and is not reimplemented here or in
		// the client. A lifter with no qualifying session comes through as zero
		// and falls out of this as level 1, which is the whole of the rule for
		// an untrained account — written once, here, rather than again in
		// TypeScript.
		p := levels.FromSessions(int(row.QualifyingSessions))
		items = append(items, lifterLevelDTO{
			LifterID:       row.UserID,
			Level:          p.Level,
			XP:             p.XP,
			XPIntoLevel:    p.XPIntoLevel,
			XPForNextLevel: p.XPForNextLevel,
		})
	}

	writeJSON(w, http.StatusOK, lifterLevelListDTO{Items: items})
}
