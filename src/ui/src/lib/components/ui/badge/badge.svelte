<script lang="ts" module>
	import { cn } from "$lib/utils.js";

	// Plain lookup rather than tailwind-variants' tv() — see button.svelte, which
	// was the other of the two callers, for why. Same caveat: this diverges from
	// shadcn-svelte's generated output for the component.
	const BADGE_BASE = "h-5 gap-1 rounded-4xl border border-transparent px-2 py-0.5 text-xs font-medium transition-all has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&>svg]:size-3! group/badge inline-flex w-fit shrink-0 items-center justify-center overflow-hidden whitespace-nowrap transition-colors focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&>svg]:pointer-events-none";

	const BADGE_VARIANTS = {
	default: "bg-primary text-primary-foreground [a]:hover:bg-primary/80",
	secondary: "bg-secondary text-secondary-foreground [a]:hover:bg-secondary/80",
	destructive: "bg-destructive/10 text-destructive focus-visible:ring-destructive/20 dark:bg-destructive/20 dark:focus-visible:ring-destructive/40 [a]:hover:bg-destructive/20",
	outline: "border-border text-foreground [a]:hover:bg-muted [a]:hover:text-muted-foreground",
	ghost: "hover:bg-muted hover:text-muted-foreground dark:hover:bg-muted/50",
	link: "text-primary underline-offset-4 hover:underline",
	} as const;

	export type BadgeVariant = keyof typeof BADGE_VARIANTS;

	export function badgeVariants({
		variant = "default",
	}: { variant?: BadgeVariant } = {}): string {
		return cn(BADGE_BASE, BADGE_VARIANTS[variant]);
	}
</script>

<script lang="ts">
	// cn comes from the module script above, which shares this component's scope.
	import { type WithElementRef } from "$lib/utils.js";
	import type { HTMLAnchorAttributes } from "svelte/elements";

	let {
		ref = $bindable(null),
		href,
		class: className,
		variant = "default",
		children,
		...restProps
	}: WithElementRef<HTMLAnchorAttributes> & {
		variant?: BadgeVariant;
	} = $props();
</script>

<svelte:element
	this={href ? "a" : "span"}
	bind:this={ref}
	data-slot="badge"
	{href}
	class={cn(badgeVariants({ variant }), className)}
	{...restProps}
>
	{@render children?.()}
</svelte:element>
