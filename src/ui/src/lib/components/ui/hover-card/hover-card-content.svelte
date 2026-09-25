<script lang="ts">
	import { LinkPreview as LinkPreviewPrimitive } from "bits-ui";
	import { cn, type WithoutChild, type WithoutChildrenOrChild } from "$lib/utils.js";
	import type { ComponentProps } from "svelte";

	let {
		ref = $bindable(null),
		class: className,
		sideOffset = 6,
		portalProps,
		...restProps
	}: WithoutChild<LinkPreviewPrimitive.ContentProps> & {
		portalProps?: WithoutChildrenOrChild<ComponentProps<typeof LinkPreviewPrimitive.Portal>>;
	} = $props();
</script>

<LinkPreviewPrimitive.Portal {...portalProps}>
	<LinkPreviewPrimitive.Content
		bind:ref
		{sideOffset}
		data-slot="hover-card-content"
		class={cn(
			"w-64 rounded-xl bg-popover p-4 text-popover-foreground ring-1 ring-foreground/10 shadow-lg duration-100 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95 z-50 outline-none",
			className
		)}
		{...restProps}
	/>
</LinkPreviewPrimitive.Portal>
