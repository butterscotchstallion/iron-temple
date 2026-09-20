// Suggested usernames for the admin's "add someone" form: lifting puns on
// famous names.
//
// Stored already in username form — lowercase, dot-separated, inside the API's
// ^[A-Za-z0-9._-]+$ and its 3..32 length — rather than as "Bench Affleck" plus a
// slugifier. The strings are written by hand either way, so a transform would
// only be a second thing that can produce something the server then rejects.
//
// A suggestion, not a decision. The field it fills is an ordinary editable
// input, and an admin who wants "dave" types "dave".
export const PUN_NAMES: readonly string[] = [
  "abdom.sandler",
  "ab.pacino",
  "abraham.lungecoln",
  "adeadlift",
  "albert.gainstein",
  "amy.puller",
  "aretha.franklift",
  "armiana.grande",
  "arnold.schwarzenlifter",
  "avril.latvigne",
  "barbell.streisand",
  "benchamin.franklin",
  "bench.affleck",
  "bill.weights",
  "britney.spotters",
  "bruce.strongsteen",
  "bruce.wheylis",
  "bulkie.eilish",
  "bulk.dylan",
  "camila.kettlebello",
  "cardio.b",
  "carrie.underhook",
  "cate.planchett",
  "celine.deload",
  "chris.hamsworth",
  "chris.rack",
  "christian.bulk",
  "christina.abguilera",
  "clark.bent",
  "clint.beastwood",
  "curlia.roberts",
  "curlize.theron",
  "david.attenbarbell",
  "diane.creatine",
  "doja.squat",
  "dr.delt",
  "drew.carrymore",
  "dua.lats",
  "dumbbelldore",
  "dumbbelly.parton",
  "dumbbell.washington",
  "ed.shredan",
  "eddie.murph",
  "elon.muscle",
  "elvis.pressley",
  "emma.squatson",
  "eva.lungeoria",
  "fifty.sets",
  "florence.push",
  "freddie.muscury",
  "gain.fonda",
  "gaining.tatum",
  "gainghis.khan",
  "gainsdalf",
  "gains.bond",
  "gains.hathaway",
  "gordon.gainsay",
  "gwyneth.pulltrow",
  "hang.solo",
  "harry.spotter",
  "hugh.jackedman",
  "ice.curls",
  "incline.jones",
  "isaac.newtons",
  "issa.rep",
  "jacked.chan",
  "jack.rack",
  "jake.gyllenhaul",
  "janet.jackedson",
  "jason.squatham",
  "jean.claude.van.gainz",
  "jenna.ortegains",
  "jennifer.abniston",
  "jennifer.lopress",
  "joan.sett",
  "johann.sebastian.back",
  "john.travolume",
  "johnny.rep",
  "judi.bench",
  "julius.curlsar",
  "justin.trimberlake",
  "kanye.chest",
  "keanu.heaves",
  "kelly.curlson",
  "kettlebell.perry",
  "kristen.dumbbell",
  "kurt.hustle",
  "leonardo.dicarbio",
  "liftzo",
  "ludwig.van.bentover",
  "lucy.lift",
  "luke.squatwalker",
  "macro.mcconaughey",
  "macros.robbie",
  "mariah.carry",
  "mark.swolberg",
  "maya.repdolph",
  "mccauley.bulkin",
  "meghan.trainor",
  "michael.gaine",
  "michelle.pfeiflex",
  "mick.jogger",
  "mindy.curling",
  "morgan.freeweight",
  "muscleangelo",
  "muscle.yeoh",
  "napoleon.bulkaparte",
  "neil.legstrong",
  "nicki.gainaj",
  "nicolas.calf",
  "oprah.winflex",
  "pablo.pecasso",
  "patrick.sweatze",
  "paul.ripped",
  "penelope.crush",
  "peter.planker",
  "plank.sinatra",
  "rachel.mcabs",
  "rephanna",
  "robert.deadniro",
  "robert.fattinson",
  "rowen.wilson",
  "ryan.gainsling",
  "ryan.repnolds",
  "saoirse.rowan",
  "seth.rowgen",
  "shania.gain",
  "shredlock.holmes",
  "sigourney.heaver",
  "squat.dogg",
  "squatlett.johansson",
  "squatwafina",
  "steve.barbell",
  "swolety.white",
  "swoley.cyrus",
  "swolma.hayek",
  "sylvester.squatlone",
  "taylor.swole",
  "theodore.rowsevelt",
  "tilda.swoleton",
  "tina.burner",
  "tom.planks",
  "tonnage.hill",
  "tony.snatch",
  "uma.thrusterman",
  "vincent.van.squat",
  "viola.deadlifts",
  "weigh.z",
  "weight.winslet",
  "wheytney.houston",
  "winston.curlchill",
  "yoked.ono",
  "zac.efform",
  "zoe.gravitz",
];

/**
 * A pun name that isn't one of `exclude`.
 *
 * `exclude` carries the roster and the name already on screen: the roster so
 * the roll can't land on a username the server would refuse as taken, and the
 * name on screen so a click always changes something — a button that sometimes
 * appears to do nothing reads as a broken button.
 *
 * Falls back to the full list when everything is excluded, since a name that
 * might collide is more use than an empty field.
 *
 * Math.random, unlike passphrase.ts next door, which draws from
 * crypto.getRandomValues. The difference is what the two produce: that one is a
 * credential, where a predictable PRNG is the whole ballgame, and this one is a
 * joke printed in a box the admin can overwrite. A username is public the
 * moment the account exists.
 */
export function randomPunName(exclude: Iterable<string> = []): string {
  // Lowercased on the way in, because uniqueness is enforced on lower(username)
  // (users_username_lower_idx) rather than on the exact string: an account
  // called "Squat.Dogg" is what makes "squat.dogg" a 409, and an exact-string
  // Set would not have noticed. Every name below is already lowercase, so this
  // only has to normalise what it is handed.
  const taken = new Set([...exclude].map((name) => name.toLowerCase()));
  const free = PUN_NAMES.filter((name) => !taken.has(name));
  const pool = free.length > 0 ? free : PUN_NAMES;
  return pool[Math.floor(Math.random() * pool.length)];
}
