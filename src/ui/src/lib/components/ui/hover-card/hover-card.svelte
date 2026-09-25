<script lang="ts">
	import { LinkPreview as LinkPreviewPrimitive } from "bits-ui";

	let {
		open = $bindable(false),
		openDelay = 200,
		closeDelay = 100,
		...restProps
	}: LinkPreviewPrimitive.RootProps = $props();
</script>

<LinkPreviewPrimitive.Root bind:open {openDelay} {closeDelay} {...restProps} />
