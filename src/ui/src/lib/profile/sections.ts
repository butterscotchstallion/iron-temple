/**
 * The profile's sections, in the order the sub-nav lists them.
 *
 * One list, read by three things: the nav that draws the pills, the route that
 * decides which panel to mount, and the guard that turns an unknown slug away.
 * Adding a section is one entry here plus its component — there is no second
 * place that has to agree about which slugs exist.
 *
 * The slug is the URL (`#/profile/equipment`), so renaming one breaks anybody's
 * bookmark. Treat them as fixed; the label is the part that may be reworded.
 */
export const PROFILE_SECTIONS = [
  { slug: "avatar", label: "Avatar" },
  { slug: "details", label: "Details" },
  { slug: "password", label: "Password" },
  { slug: "equipment", label: "Equipment" },
  { slug: "achievements", label: "Achievements" },
  { slug: "data", label: "Your data" },
] as const;

export type ProfileSection = (typeof PROFILE_SECTIONS)[number]["slug"];

/**
 * Where a bare `#/profile` lands.
 *
 * Details rather than the first pill: it is the section a lifter opens the
 * screen for, and the one whose contents (name, chip colour) they are most
 * likely to be looking for. Avatar leads the nav because it is the most visual,
 * not because it is the most used.
 */
export const DEFAULT_SECTION: ProfileSection = "details";

/** Narrows a URL fragment to a section, or null if it names none. */
export function toSection(slug: string | undefined): ProfileSection | null {
  return (
    PROFILE_SECTIONS.find((section) => section.slug === slug)?.slug ?? null
  );
}
