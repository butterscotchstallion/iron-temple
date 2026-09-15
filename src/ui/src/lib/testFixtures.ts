/**
 * Fixtures the unit tests share.
 *
 * Seven test files were each building the same signed-in lifter by hand, which
 * is fine until a required field is added to User: then seven files fail to
 * compile and get seven separate hand-applied fixes, and any that a test happens
 * not to typecheck against goes quietly stale instead.
 *
 * Only the identity lives here. Anything a test is actually asserting about —
 * the bar and rack ExerciseCard's warm-up arithmetic assumes, the gym a profile
 * test signs in with — stays in that file as an override, where its reasoning
 * can be read next to the assertions that depend on it.
 */

import type { User } from "./api";

/**
 * A signed-in lifter. An admin, because the first account to register claims the
 * install, so the ordinary case in this app IS the admin — and with no avatar,
 * so components take the initials path unless a test says otherwise.
 */
export function testUser(overrides: Partial<User> = {}): User {
  return {
    id: 1,
    username: "ada",
    displayName: "Ada Lovelace",
    avatarColor: "",
    isAdmin: true,
    hasAvatar: false,
    ...overrides,
  };
}
