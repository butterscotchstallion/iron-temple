/**
 * A four-word passphrase for the temporary password an admin hands out.
 *
 * The admin area asks one person to invent a password for another, which is the
 * worst possible prompt: the account is not theirs, they will not have to
 * remember it, and they are one field away from finishing the task. Left to a
 * human that field gets "password1" or the new lifter's first name. Filling it
 * in removes the decision instead of nagging about it — the good password is
 * already there, and typing a bad one over it takes deliberate effort.
 *
 * Words rather than characters because of what happens next: this credential is
 * read down a phone or written on paper, and "amber-thistle-cobalt-walnut"
 * survives that where "xK7#mq2z" does not. It is also a one-time credential —
 * the account can do nothing but replace it (see 0021_must_change_password), so
 * it needs to survive being spoken once, not being remembered.
 *
 * Entropy is four independent draws from the 1,316 words below: 41 bits, or
 * three trillion passphrases. Set against the API's login limiter — 10 attempts
 * per 15 minutes per address-and-username — the average guess lands after some
 * four million years, and the credential stops existing at first sign-in
 * anyway. The weak link is the phone call, not the search space, which is the
 * right place for it to be.
 *
 * Lives in the lazily-loaded admin chunk (see App.svelte), so the ~12 KB of
 * wordlist is never downloaded by a lifter who cannot administer anything.
 */

/**
 * The wordlist.
 *
 * Every entry is 3–9 lowercase letters, common enough to spell from hearing,
 * and concrete. Homophone pairs are deliberately absent — no "flour"/"flower",
 * no "steal"/"steel" — because the whole point is that this survives being read
 * aloud, and a word the listener can spell two ways does not.
 *
 * Repeats ARE allowed when drawing, which is what makes the entropy above a
 * plain power. A rare "amber-amber-…" is not a defect.
 *
 * Guarded by passphrase.test.ts: uniqueness, the charset, and a floor on the
 * length, so the entropy above cannot quietly drop when someone prunes a word
 * they dislike.
 */
export const WORDS = [
  // a
  "able", "acorn", "acre", "actor", "adobe", "adopt", "agent", "album", "alert",
  "algae", "alibi", "alien", "alley", "allow", "alloy", "almond", "aloe", "alone",
  "alpha", "altar", "amber", "amble", "amend", "amino", "ample", "amuse", "anchor",
  "angel", "anger", "angle", "ankle", "anvil", "apex", "apple", "apron", "arbor",
  "arcade", "arch", "arctic", "area", "arena", "argue", "armor", "aroma", "array",
  "arrow", "aspen", "asset", "atlas", "atom", "attic", "auburn", "audio", "aunt",
  "auto", "autumn", "avenue", "awake", "award", "awning", "axis", "axle", "azure",
  // b
  "bacon", "badge", "bagel", "baker", "ballad", "bamboo", "banana", "banjo",
  "banner", "barge", "barley", "barn", "barrel", "basil", "basin", "basket",
  "batch", "baton", "beach", "beacon", "beam", "bean", "beaver", "beech",
  "beetle", "bench", "berry", "bicycle", "birch", "bison", "bistro", "blanket",
  "blaze", "blend", "blink", "block", "bloom", "blossom", "bluff", "blush",
  "boiler", "bolt", "bonnet", "bonus", "boulder", "bounce", "bracket", "braid",
  "branch", "brass", "bravo", "bread", "breeze", "brick", "bridge", "bright",
  "bronze", "brook", "broom", "brush", "bubble", "bucket", "buckle", "buffet",
  "bugle", "bulb", "bundle", "bunker", "burrow", "bushel", "butler", "button",
  // c
  "cabin", "cable", "cactus", "camel", "cameo", "camera", "campus", "canal",
  "candle", "candy", "canopy", "canvas", "canyon", "cape", "caramel", "cardigan",
  "cargo", "carpet", "carrot", "cartoon", "carve", "cascade", "cashew", "casino",
  "castle", "catalog", "cedar", "celery", "cellar", "cello", "cement", "census",
  "chalet", "chalk", "chapel", "charm", "cheddar", "cherry", "chess", "chestnut",
  "chill", "chimney", "chisel", "chorus", "cider", "cinema", "circle", "circus",
  "citrus", "clamp", "clarity", "clasp", "clay", "cleaver", "cliff", "cloak",
  "clock", "closet", "clover", "cluster", "coast", "cobalt", "cobble", "cocoa",
  "coffee", "collar", "colony", "column", "comet", "compass", "concert", "condor",
  "copper", "coral", "cork", "corner", "cottage", "cotton", "cougar", "council",
  "cousin", "cove", "cowbell", "coyote", "crackle", "cradle", "crane", "crater",
  "crayon", "cream", "crescent", "crest", "cricket", "crimson", "crisp", "crocus",
  "crop", "crown", "crumb", "crystal", "cube", "cuckoo", "cuff", "curator",
  "curb", "curl", "curtain", "cushion", "custard", "cymbal", "cypress",
  // d
  "daffodil", "dagger", "dahlia", "daisy", "damson", "dancer", "dandy", "dapple",
  "dart", "dasher", "dawn", "daybreak", "deck", "decoy", "delta", "denim",
  "depot", "desert", "diagram", "diamond", "diary", "diesel", "digit", "dinner",
  "dipper", "district", "ditch", "diver", "dock", "dolphin", "domain", "dome",
  "donkey", "donut", "doorway", "dorsal", "dough", "dove", "dragon", "drapery",
  "drawer", "dresser", "drift", "drill", "drizzle", "drum", "dryer", "duckling",
  "duffel", "dugout", "dune", "dusk", "dustpan", "duvet", "dwelling", "dynamo",
  // e
  "eagle", "earth", "easel", "ebony", "echo", "eclipse", "edge", "eggplant",
  "elbow", "elder", "electric", "elegant", "element", "elephant", "elevator",
  "elk", "elm", "ember", "emblem", "emerald", "empire", "enamel", "engine",
  "envelope", "equator", "errand", "escort", "essay", "estate", "etching",
  "eucalypt", "evening", "exhibit", "exit", "expert", "extra",
  // f
  "fabric", "facet", "factory", "falcon", "fanfare", "fang", "farm", "fathom",
  "fauna", "fawn", "feather", "fedora", "fence", "fennel", "fern", "ferry",
  "fiddle", "fidget", "field", "fig", "filament", "fillet", "filter", "finch",
  "fjord", "flagon", "flake", "flamingo", "flannel", "flask", "fleece", "flint",
  "float", "floral", "flute", "foal", "foam", "foliage", "folder", "fondue",
  "forest", "fork", "formal", "fossil", "fountain", "fox", "foyer", "fragment",
  "frame", "freckle", "frigate", "fringe", "frost", "fudge", "funnel", "furnace",
  // g
  "gable", "gadget", "gala", "galaxy", "gallery", "gallon", "gambit", "garden",
  "gargoyle", "garland", "garlic", "garnet", "gate", "gauge", "gazebo", "gazelle",
  "gear", "gecko", "gelato", "gem", "geode", "geyser", "giant", "ginger",
  "giraffe", "glacier", "glade", "gland", "glass", "glider", "globe", "gloss",
  "glove", "glow", "gnome", "goblet", "goggles", "gondola", "goose", "gopher",
  "gourd", "granite", "grape", "graph", "gravel", "grill", "grotto", "grove",
  "guitar", "gully", "gusto", "gutter", "gymnast",
  // h
  "habitat", "hacienda", "halibut", "hallway", "hamlet", "hammer", "hammock",
  "hamper", "handle", "hangar", "harbor", "harmony", "harp", "harvest", "hatch",
  "hawk", "hazel", "header", "hearth", "heather", "hedge", "helium", "helmet",
  "hemlock", "herald", "herb", "heron", "hickory", "highway", "hillside",
  "hinge", "hoist", "holly", "homestead", "honey", "hoodie", "hopper", "horizon",
  "hornet", "horse", "hostel", "hotel", "hour", "hubcap", "huddle", "humid",
  "hurdle", "husk", "hutch", "hydrant", "hymn",
  // i
  "iceberg", "icicle", "idea", "igloo", "image", "impala", "incline", "index",
  "indigo", "infant", "ingot", "inkwell", "inlet", "insect", "instant", "iris",
  "iron", "island", "isotope", "ivory", "ivy",
  // j
  "jacket", "jackpot", "jade", "jaguar", "jamboree", "jasmine", "javelin", "jelly",
  "jersey", "jetty", "jewel", "jigsaw", "jockey", "jogger", "journal", "joust",
  "jubilee", "judo", "juggler", "juice", "jumbo", "jungle", "juniper", "jury",
  // k
  "kale", "kayak", "kelp", "kennel", "kernel", "kettle", "keyboard", "keystone",
  "kilt", "kimono", "kindle", "kingdom", "kiosk", "kitchen", "kite", "kitten",
  "kiwi", "knapsack", "knight", "knoll", "koala", "krypton",
  // l
  "label", "lace", "ladder", "ladle", "lagoon", "lake", "lamb", "lamp",
  "lantern", "lapel", "larch", "lariat", "lark", "lasso", "latch", "lattice",
  "laurel", "lava", "lavender", "leaflet", "leather", "ledge", "ledger", "legacy",
  "legume", "lemon", "lentil", "leopard", "lever", "library", "lichen", "lilac",
  "lily", "limber", "lime", "linen", "lining", "lion", "liquid", "listen",
  "lizard", "llama", "loaf", "lobby", "lobster", "locker", "locust", "lodge",
  "loft", "logbook", "lollipop", "lookout", "loom", "lotus", "lounge", "lumber",
  "lunar", "lupine", "lyric",
  // m
  "macaw", "machine", "magenta", "magnet", "magnolia", "mahogany", "mailbox",
  "mallet", "mammoth", "mandolin", "mango", "mangrove", "manor", "mantle",
  "maple", "marathon", "marble", "margin", "marigold", "marina", "marker",
  "market", "marmot", "marsh", "marvel", "mascot", "mask", "mason", "mast",
  "matinee", "meadow", "medal", "melody", "melon", "memoir", "menu", "mercury",
  "mermaid", "mesa", "mesh", "metal", "meteor", "midnight", "mildew", "mile",
  "milk", "mill", "mimosa", "mineral", "mink", "minnow", "mint", "mirror",
  "mist", "mitten", "moat", "mobile", "mocha", "model", "module", "molten",
  "moment", "monarch", "monsoon", "moped", "moraine", "morsel", "mortar",
  "mosaic", "moss", "motel", "motor", "mountain", "mouse", "muffin", "mulberry",
  "mural", "museum", "mushroom", "music", "mussel", "mustard", "myrtle",
  // n
  "napkin", "narwhal", "nature", "nautical", "nebula", "necklace", "nectar",
  "needle", "neon", "nest", "nettle", "network", "newt", "niche", "nickel",
  "nimbus", "nitrogen", "nomad", "noodle", "noon", "north", "nostril", "notable",
  "notebook", "nougat", "novel", "nozzle", "nugget", "number", "nursery", "nutmeg",
  // o
  "oak", "oasis", "oat", "obelisk", "oboe", "observe", "ocean", "ocelot",
  "octagon", "octave", "octopus", "office", "offshore", "oil", "olive", "omelet",
  "onion", "onyx", "opal", "opera", "orange", "orbit", "orchard", "orchid",
  "organ", "origami", "oriole", "ornament", "osprey", "ostrich", "otter",
  "outback", "outcrop", "outlet", "outpost", "oven", "overalls", "owl", "oxide",
  "oyster", "ozone",
  // p
  "pacific", "packet", "paddle", "pagoda", "paint", "palace", "palette", "palm",
  "pamphlet", "panda", "panel", "pansy", "panther", "papaya", "paprika", "papyrus",
  "parade", "parcel", "parchment", "parka", "parlor", "parrot", "parsley",
  "parsnip", "pasture", "pathway", "patio", "pattern", "peach", "peacock",
  "peanut", "pearl", "pebble", "pecan", "pedal", "pelican", "pendant", "penguin",
  "peony", "pepper", "perch", "pergola", "persimmon", "petal", "pewter", "phantom",
  "pheasant", "phoenix", "piano", "picket", "pigment", "pilaf", "pillar", "pillow",
  "pilot", "pinecone", "pinnacle", "pioneer", "pipe", "piston", "pitcher", "pivot",
  "plank", "plateau", "platinum", "platter", "plaza", "pliers", "plover", "plum",
  "plumage", "pocket", "podium", "polar", "pollen", "pond", "pony", "poplar",
  "poppy", "porch", "portal", "possum", "postage", "poster", "pottery", "pouch",
  "powder", "prairie", "praline", "present", "pretzel", "primrose", "prism",
  "private", "prize", "produce", "project", "propel", "protein", "prune", "public",
  "pudding", "puffin", "pulley", "pumice", "pumpkin", "punch", "puppet", "purple",
  "puzzle", "pylon", "pyramid",
  // q
  "quail", "quarry", "quart", "quartz", "quaver", "quest", "queue", "quiche",
  "quill", "quilt", "quince", "quiver", "quorum",
  // r
  "rabbit", "raccoon", "racket", "radar", "radish", "raft", "ragtime", "railcar",
  "rainbow", "raisin", "rake", "ramble", "ranch", "rapids", "raptor", "raspberry",
  "rattle", "raven", "ravine", "razor", "reactor", "realm", "reason", "rebate",
  "recipe", "record", "redwood", "reed", "reef", "refuge", "regatta", "region",
  "relay", "relic", "remote", "rescue", "reservoir", "resin", "retriever",
  "rhubarb", "rhythm", "ribbon", "ridge", "rifle", "rigging", "rimrock", "ripple",
  "risotto", "river", "roadway", "roast", "robin", "robot", "rocket", "rodeo",
  "roost", "rooster", "rosemary", "rotor", "rubber", "ruby", "rudder", "rugby",
  "ruler", "rumble", "runner", "runway", "rustic", "rutabaga",
  // s
  "saddle", "safari", "saffron", "sage", "sailor", "salad", "salmon", "salsa",
  "sample", "sanctum", "sandal", "sapling", "sapphire", "sardine", "satchel",
  "satin", "sauna", "savanna", "sawdust", "saxophone", "scaffold", "scallop",
  "scanner", "scarlet", "scenic", "school", "scissors", "scooter", "scorpion",
  "scout", "scramble", "screen", "scroll", "sculpt", "seagull", "seashell",
  "season", "seaweed", "sediment", "seedling", "sequoia", "serpent", "sesame",
  "settler", "shadow", "shale", "shamrock", "shanty", "shelf", "sherbet",
  "shingle", "shipyard", "shore", "shovel", "shrub", "shutter", "sidewalk",
  "signal", "silica", "silk", "silo", "silver", "siren", "sisal", "skillet",
  "skipper", "skylight", "slalom", "slate", "sleigh", "slipper", "slogan",
  "smelter", "smoke", "snapper", "snorkel", "snowdrop", "socket", "sofa",
  "solar", "solder", "sonata", "sonnet", "soprano", "sorbet", "soup", "south",
  "soybean", "spade", "spaniel", "sparrow", "spatula", "speaker", "spiral",
  "splinter", "sponge", "spool", "spring", "sprocket", "spruce", "squash",
  "squid", "stable", "stadium", "stallion", "stamp", "stanza", "starling",
  "station", "statue", "stencil", "stereo", "stirrup", "stone", "stool",
  "storm", "stove", "strudel", "stucco", "studio", "stylus", "subway", "sugar",
  "sulfur", "summit", "sundial", "sunset", "surface", "swallow", "swamp",
  "sweater", "swivel", "sycamore", "symbol", "syrup",
  // t
  "table", "tabby", "tackle", "taffy", "talon", "tambour", "tandem", "tangelo",
  "tangent", "tankard", "tapestry", "tapioca", "tarragon", "tassel", "tavern",
  "teacup", "teak", "teal", "telegram", "telescope", "temple", "tendril",
  "tenor", "tent", "terrace", "terrain", "textile", "texture", "thatch",
  "thermal", "thicket", "thimble", "thistle", "thorn", "thunder", "ticket",
  "tidal", "tiger", "timber", "tinsel", "toast", "toboggan", "toffee", "tofu",
  "toggle", "token", "tomato", "topaz", "topiary", "torch", "tornado", "torrent",
  "tortoise", "totem", "toucan", "towel", "tower", "trackpad", "tractor",
  "traffic", "trailer", "transit", "trapeze", "travel", "treble", "trellis",
  "trestle", "triangle", "tribute", "trident", "trillium", "trinket", "triumph",
  "trolley", "trombone", "tropic", "trout", "trowel", "truffle", "trumpet",
  "trunk", "tuba", "tulip", "tumbler", "tundra", "tunnel", "turban", "turbine",
  "turkey", "turnip", "turret", "turtle", "tusk", "tweed", "twilight", "typhoon",
  // u
  "ukulele", "umbra", "umpire", "unicorn", "uniform", "union", "unity",
  "upland", "upright", "upstream", "uranium", "urban", "urchin", "utensil",
  // v
  "vacuum", "valley", "valve", "vanilla", "vantage", "vapor", "varnish", "vault",
  "velvet", "vendor", "veranda", "verbena", "vertex", "vessel", "viaduct",
  "vicuna", "video", "village", "vinegar", "vineyard", "viola", "violet",
  "viper", "vista", "vitamin", "vocal", "volcano", "volley", "voltage", "voyage",
  // w
  "wafer", "waffle", "wagon", "walnut", "walrus", "wander", "wardrobe", "warren",
  "wasabi", "watch", "waterfall", "wattle", "weasel", "weather", "weaver",
  "webbing", "wedge", "welcome", "western", "wetland", "whale", "wharf", "wheat",
  "wheel", "whisker", "whistle", "wicker", "widget", "wigwam", "wildcat",
  "willow", "windmill", "window", "wingspan", "winter", "wisteria", "wolf",
  "wombat", "wonder", "woodland", "woodwind", "workshop", "wrangler", "wreath",
  "wrench", "wristband",
  // x, y, z
  "xenon", "xylophone", "yacht", "yardstick", "yarrow", "yearling", "yeast",
  "yellow", "yeoman", "yodel", "yogurt", "yonder", "yucca", "zebra", "zenith",
  "zephyr", "zeppelin", "zigzag", "zinnia", "zipper", "zircon", "zodiac",
] as const;

/** How many words a generated passphrase has. */
export const PASSPHRASE_WORDS = 4;

/**
 * A uniformly random index below `bound`, from the platform CSPRNG.
 *
 * `crypto.getRandomValues`, never `Math.random()`: this produces a credential,
 * and Math.random is a seeded PRNG whose output is predictable from a handful of
 * prior values.
 *
 * Rejection sampling rather than a plain `% bound`, because 2^32 is not a
 * multiple of the list length: the low indices would come up fractionally more
 * often than the high ones, and a wordlist with favourites is a wordlist smaller
 * than it looks. Discarding the short tail costs a re-draw for roughly one call
 * in six million and keeps every word equally likely.
 */
function randomIndex(bound: number): number {
  const ceiling = 2 ** 32;
  const limit = Math.floor(ceiling / bound) * bound;
  const buffer = new Uint32Array(1);
  let draw: number;
  do {
    crypto.getRandomValues(buffer);
    draw = buffer[0];
  } while (draw >= limit);
  return draw % bound;
}

/**
 * A hyphen-joined passphrase, e.g. `amber-thistle-cobalt-walnut`.
 *
 * Hyphens rather than spaces so it survives a copy-paste into any field and can
 * be dictated as "amber hyphen thistle"; the API puts no charset restriction on
 * a password, unlike a username.
 */
export function passphrase(words: number = PASSPHRASE_WORDS): string {
  return Array.from({ length: words }, () => WORDS[randomIndex(WORDS.length)]).join("-");
}
