<script lang="ts">
  import Anchor from "@lucide/svelte/icons/anchor";
  import Anvil from "@lucide/svelte/icons/anvil";
  import Castle from "@lucide/svelte/icons/castle";
  import Dumbbell from "@lucide/svelte/icons/dumbbell";
  import Flame from "@lucide/svelte/icons/flame";
  import Gem from "@lucide/svelte/icons/gem";
  import Hammer from "@lucide/svelte/icons/hammer";
  import Landmark from "@lucide/svelte/icons/landmark";
  import Mountain from "@lucide/svelte/icons/mountain";
  import Shield from "@lucide/svelte/icons/shield";
  import Skull from "@lucide/svelte/icons/skull";
  import Star from "@lucide/svelte/icons/star";
  import Swords from "@lucide/svelte/icons/swords";
  import Target from "@lucide/svelte/icons/target";
  import Trophy from "@lucide/svelte/icons/trophy";
  import Zap from "@lucide/svelte/icons/zap";
  import type { House, HouseIcon as HouseIconName } from "./api";
  import { avatarColor } from "./userAvatar";

  // A House's icon, in its accent colour.
  //
  // ONE MAP, and it is typed by the generated HouseIcon union. That is the point
  // of the icon being a closed enum in openapi.yaml rather than a free string: a
  // name added to the spec and not to this map is a type error here, where a free
  // string would have been a House that silently draws nothing.
  //
  // The empty string is a member of that union — a House founded without an icon
  // has it — and this renders nothing for it. Callers fall back to the sigil,
  // which every House has.
  const ICONS: Record<Exclude<HouseIconName, "">, typeof Dumbbell> = {
    dumbbell: Dumbbell,
    flame: Flame,
    anvil: Anvil,
    hammer: Hammer,
    mountain: Mountain,
    shield: Shield,
    swords: Swords,
    skull: Skull,
    zap: Zap,
    star: Star,
    gem: Gem,
    landmark: Landmark,
    castle: Castle,
    trophy: Trophy,
    target: Target,
    anchor: Anchor,
  };

  type IconHouse = Pick<House, "id" | "icon" | "iconColor">;

  let {
    house,
    size = "size-5",
  }: {
    house: IconHouse;
    /**
     * Tailwind size class. Surfaces differ a lot — a 16px sigil chip and a 40px
     * page header both use this — and an icon sized for one is lost in the other.
     */
    size?: string;
  } = $props();

  const Icon = $derived(house.icon === "" ? null : ICONS[house.icon]);
  // The same derivation the initials chip uses, keyed on the House's id rather
  // than a lifter's: a House with no colour chosen still gets a stable one, and
  // it is the same one every time it is drawn.
  const color = $derived(avatarColor(house.id, house.iconColor));
</script>

{#if Icon}
  <Icon class="{size} shrink-0" style="color:{color}" aria-hidden="true" />
{/if}
