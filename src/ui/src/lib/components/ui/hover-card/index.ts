import Content from "./hover-card-content.svelte";
import Trigger from "./hover-card-trigger.svelte";
import Root from "./hover-card.svelte";

// bits-ui calls this primitive LinkPreview; it is wrapped under the name the rest
// of the ecosystem uses, because what it does here is preview a House rather than
// a link, and "link preview" would send a reader looking for an anchor.
export {
	Root,
	Trigger,
	Content,
	//
	Root as HoverCard,
	Trigger as HoverCardTrigger,
	Content as HoverCardContent,
};
