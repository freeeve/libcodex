package bibframe

// iso6391to2b maps an ISO 639-1 two-letter language code to its MARC 21 / ISO
// 639-2/B three-letter code -- the bibliographic variant id.loc.gov's language
// vocabulary uses. Some ILS exports (notably Koha records harvested via OAI-PMH)
// carry a 639-1 code in 008/35-37 or 041 where MARC 21 requires the 639-2 code, so
// the forward crosswalk would otherwise drop the language. This table normalizes
// them. Verified against LoC's conf/iso6392-to-1.xml, which is marc2bibframe2's
// active language map (variables.xsl loads it; the similarly named
// languageCrosswalk.xml is dead -- its loader is commented out -- and lists the /T
// code, so it is the wrong source to mine). Every entry's 639-2 code maps back to
// its 639-1 in that file, and the ~20 languages whose bibliographic and
// terminologic codes differ use the /B code (sq -> alb, not sqi) so the language
// IRI resolves at id.loc.gov/vocabulary/languages.
var iso6391to2b = map[string]string{
	"aa": "aar", "ab": "abk", "af": "afr", "ak": "aka", "sq": "alb",
	"am": "amh", "ar": "ara", "an": "arg", "hy": "arm", "as": "asm",
	"av": "ava", "ae": "ave", "ay": "aym", "az": "aze", "ba": "bak",
	"bm": "bam", "eu": "baq", "be": "bel", "bn": "ben", "bh": "bih",
	"bi": "bis", "bo": "tib", "bs": "bos", "br": "bre", "bg": "bul",
	"my": "bur", "ca": "cat", "cs": "cze", "ch": "cha", "ce": "che",
	"zh": "chi", "cu": "chu", "cv": "chv", "kw": "cor", "co": "cos",
	"cr": "cre", "cy": "wel", "da": "dan", "de": "ger", "dv": "div",
	"nl": "dut", "dz": "dzo", "el": "gre", "en": "eng", "eo": "epo",
	"et": "est", "ee": "ewe", "fo": "fao", "fa": "per", "fj": "fij",
	"fi": "fin", "fr": "fre", "fy": "fry", "ff": "ful", "ka": "geo",
	"gd": "gla", "ga": "gle", "gl": "glg", "gv": "glv", "gn": "grn",
	"gu": "guj", "ht": "hat", "ha": "hau", "he": "heb", "hz": "her",
	"hi": "hin", "ho": "hmo", "hr": "hrv", "hu": "hun", "ig": "ibo",
	"is": "ice", "io": "ido", "ii": "iii", "iu": "iku", "ie": "ile",
	"ia": "ina", "id": "ind", "ik": "ipk", "it": "ita", "jv": "jav",
	"ja": "jpn", "kl": "kal", "kn": "kan", "ks": "kas", "kr": "kau",
	"kk": "kaz", "km": "khm", "ki": "kik", "rw": "kin", "ky": "kir",
	"kv": "kom", "kg": "kon", "ko": "kor", "kj": "kua", "ku": "kur",
	"lo": "lao", "la": "lat", "lv": "lav", "li": "lim", "ln": "lin",
	"lt": "lit", "lb": "ltz", "lu": "lub", "lg": "lug", "mk": "mac",
	"mh": "mah", "ml": "mal", "mi": "mao", "mr": "mar", "ms": "may",
	"mg": "mlg", "mt": "mlt", "mn": "mon", "na": "nau", "nv": "nav",
	"nr": "nbl", "nd": "nde", "ng": "ndo", "ne": "nep", "nn": "nno",
	"nb": "nob", "no": "nor", "ny": "nya", "oc": "oci", "oj": "oji",
	"or": "ori", "om": "orm", "os": "oss", "pa": "pan", "pi": "pli",
	"pl": "pol", "pt": "por", "ps": "pus", "qu": "que", "rm": "roh",
	"ro": "rum", "rn": "run", "ru": "rus", "sg": "sag", "sa": "san",
	"si": "sin", "sk": "slo", "sl": "slv", "se": "sme", "sm": "smo",
	"sn": "sna", "sd": "snd", "so": "som", "st": "sot", "es": "spa",
	"sc": "srd", "sr": "srp", "ss": "ssw", "su": "sun", "sw": "swa",
	"sv": "swe", "ty": "tah", "ta": "tam", "tt": "tat", "te": "tel",
	"tg": "tgk", "tl": "tgl", "th": "tha", "ti": "tir", "to": "ton",
	"tn": "tsn", "ts": "tso", "tk": "tuk", "tr": "tur", "tw": "twi",
	"ug": "uig", "uk": "ukr", "ur": "urd", "uz": "uzb", "ve": "ven",
	"vi": "vie", "vo": "vol", "wa": "wln", "wo": "wol", "xh": "xho",
	"yi": "yid", "yo": "yor", "za": "zha", "zu": "zul",
}

// normalizeLang returns the MARC 639-2/B code for a raw language code from a 008 or
// 041: a valid three-letter code is returned as-is, a two-letter 639-1 code is
// mapped to its 639-2/B code, and anything else yields "" so the caller drops it
// rather than emit an unresolvable language IRI.
func normalizeLang(code string) string {
	switch {
	case isLangCode(code):
		return code
	case len(code) == 2:
		return iso6391to2b[code]
	default:
		return ""
	}
}
