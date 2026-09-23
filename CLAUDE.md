# CLAUDE.md

@AGENTS.md

---

**This file is an import shim, and that is all it is.** Every convention for this
repo lives in [`AGENTS.md`](AGENTS.md); the line above pulls it into the session.

It exists because a bare `AGENTS.md` is only loaded automatically by some Claude
Code versions, whereas an `@`-import from `CLAUDE.md` is loaded unconditionally.
Without it, whether an agent sees this repo's rules — conventional commits, the
generated-client rules, the PR watch loop — depended on which version the
operator happened to be running, and a rule that silently isn't loaded is
indistinguishable from a rule nobody wrote.

**Add guidance to `AGENTS.md`, not here.** The AI reviewer reads *both* files and
injects them verbatim into its prompt (`ai_pr_review.read_conventions`), so
anything duplicated here is sent twice and gets two chances to drift.
