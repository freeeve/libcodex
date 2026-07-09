# BIBFRAME crosswalk vs LC marc2bibframe2 -- prior-art audit

Task 059. Audits the libcodex BIBFRAME crosswalk (`bibframe/`) against LC's
official `marc2bibframe2` XSLT converter, area by area, to record where we match
prior art, where we deliberately simplify, and where we have gaps. Each actionable
gap is filed as a numbered task (see the register at the end).

Method: six parallel readers, each comparing one subsystem of our forward
(`FromRecord`/`shape.go`) and reverse (`reader_crosswalk.go`) crosswalk against the
matching `ConvSpec-*.xsl` module, fetched via
`gh api repos/lcnetdev/marc2bibframe2/contents/xsl/<file> --jq .content | base64 -d`.

Classification: **MATCH** (we do what m2b does), **DIVERGENCE** (intentional
simplification, defensible), **GAP** (m2b does something we don't) with severity.

Note on m2b's architecture: current m2b routes uniform titles and name-title
entries through `bf:Hub` nodes and models subject subdivisions as
`madsrdf:ComplexSubject`. libcodex intentionally uses a flatter, label-oriented
model (no Hub, `--`-joined subject labels). Several "divergences" below are this
one deliberate posture; the tasks preserve it and add the cheap, faithful signals
(sources, statuses, vocab IRIs) that don't require the Hub/MADS machinery.

---

## 1. Titles (245 / 130 / 240 / 246)

Summary: the 245 -> Instance path (mainTitle/subtitle/partNumber/partName from
$a/$b/$n/$p, $c -> responsibilityStatement) matches. Gaps are completeness:
nonfiling, 246 variants, uniform-title subfields.

- MATCH -- 245 -> Instance `bf:Title` with $a/$b/$n/$p split; $c ->
  `bf:responsibilityStatement`. (ConvSpec-200-247not240-Titles.xsl, match 245.)
- DIVERGENCE (low) -- `trimISBD` chops one trailing `/ : ; ,`; m2b `tChopPunct`
  also strips trailing periods. End-of-field periods survive in our labels.
- RESOLVED [071] -- 245 ind2 (1-9) -> `bflc:nonSortNum` on the Title, reversed to
  the 245 second indicator.
- DIVERGENCE (med) -- m2b always emits a Work `bf:title` from 245; we put the
  transcribed title on the Work only when there is no 130/240. Defensible.
- DIVERGENCE (med, structural) -- 130/240 uniform title emitted as a direct
  `bf:title` on the Work; m2b routes it through `bf:expressionOf -> bf:Hub`.
  Deliberate (pre-Hub model).
- RESOLVED [071] -- 130/240 now carry $n/$p (partNumber/partName). $l/$f/$s/$m/$r/$o
  still dropped (deferred, low-frequency).
- RESOLVED [071] -- 246 -> `bf:VariantTitle` (or `bf:ParallelTitle` for ind2=1) under
  `bf:title`, with a `bf:variantType` token from ind2; cover(4)/spine(8) attach to
  the Instance, the rest to the Work. Reverse restores the 246 indicator. The main-
  title reader (`firstTitle`) skips variant/parallel-typed nodes.
- GAP (low), deferred [071] -- 210/222/242/243/247 and 245 $f/$g/$s unhandled.

## 2. Contributions / names (1XX / 7XX)

Summary: structure matches (`bf:contribution` -> `bf:Contribution` + `bf:agent`,
1xx primary), but agents are untyped-IRI blank nodes labeled from $a only and
roles are always literals -- controlled-vocabulary content is lost.

- MATCH -- `bf:contribution`/`bf:Contribution` + `bf:agent`/`bf:role`; 1xx typed
  `bflc:PrimaryContribution`. (ConvSpec-1XX,7XX,8XX-names.xsl, mode workName.)
- GAP (high) -- $4 relator code never mapped to a
  `.../vocabulary/relators/<code>` IRI; role emitted as a bare `rdfs:label`. **[061]**
- DIVERGENCE (weak) -- role source order: we prefer $e then $4; m2b prefers $4
  (controlled) then $e. Adopt m2b's order. **[061]**
- GAP (med) -- multi/compound roles collapsed to first $e; m2b iterates all $e/$4
  and splits on `, and &`. **[061]**
- GAP (med) -- agent ind1 not consulted: x00 ind1=3 -> `bf:Family`, x10 ind1=1 ->
  `bf:Jurisdiction` downgraded to Person/Organization. **[061]**
- GAP (med) -- agent label from $a only; m2b concatenates the tag-appropriate set
  ($a$b$c$d$q... ) so dates/fuller-form/subordinate units are dropped. **[061]**
- GAP (med) -- $0/$1 authority URIs dropped (no agent IRI / authority link). **[061]**
- GAP (med/high) -- 7xx carrying $t (name-title) built as a spurious Contribution;
  m2b routes it to a related work (`bf:relation`/Hub). **[062]**
- DIVERGENCE (defensible) -- m2b always attaches a default role (aut/ctb); we emit
  none when $e/$4 absent.
- GAP (low) -- 111/711 relator should read $j not $e; 720 uncontrolled names, 880
  vernacular, and $5-only 7xx suppression unhandled. **[061]**

## 3. Subjects & genre (6XX)

Summary: simple-case shape matches (`bf:subject` + typed node, `bf:genreForm`),
but subdivided headings collapse to one `--` label and no `bf:source` (thesaurus)
is ever emitted.

- MATCH -- `bf:subject` -> `bf:Topic`/`bf:Person`/`bf:Organization`/`bf:Meeting`
  for the non-subdivided case. (ConvSpec-600-662.xsl, work6XXAuth.)
- DIVERGENCE (high, deliberate) -- $a/$x/$y/$z/$v joined with `--` into one
  `rdfs:label`; m2b builds `madsrdf:ComplexSubject` + `madsrdf:componentList` with
  per-subfield-typed parts. Keep the flat label (posture), but fix the two cheap
  losses below. **[060]**
- GAP (med) -- no `bf:source` on any subject; m2b derives it from ind2
  (0=LCSH, 2=MeSH, ...) and $2. Mirror our existing `Classification.Source`. **[060]**
- DIVERGENCE (defensible) -- reverse re-splits the `--` label to $a+$x* with
  hardcoded ind2=0, so $y/$z/$v and non-LCSH thesauri do not round-trip. Preserve
  subdivision codes + source to fix. **[060]**
- DIVERGENCE/GAP (med) -- 655 with $v/$x/$y/$z misfiled as a flat `bf:genreForm`;
  m2b treats a subdivided 655 as a subject. No genre `bf:source` (lcgft) either. **[060]**
- DIVERGENCE (low) -- 651 always `bf:Place`; m2b uses `bf:Topic` when subdivided.
  600 ind1=3/610 ind1=1 Family/Jurisdiction dropped.
- GAP (low) -- 648/653/647/656/657/662, 6xx $t name-title, $0/$1 unhandled. **[060]**

## 4. Identifiers & Classification (0XX)

Summary: common typing matches (020->Isbn, 022->Issn, 050->Lcc, 082->Ddc) and the
ISBN qualifier (058) is faithful; gaps are the secondary signals -- status,
024-ind1 scheme, LCCN producer, item portions/assigners.

- MATCH -- 020->`bf:Isbn`, 022->`bf:Issn`, 050->`bf:ClassificationLcc`,
  082->`bf:ClassificationDdc`, 084 repeated $a, ISBN qualifier.
- GAP (med/high) -- $z/$y canceled/invalid on 020/022/024 dropped; m2b emits
  `bf:status -> bf:Status` (cancinv/incorrect). No `Identifier.Status` field. **[063]**
- GAP (high) -- 024 ind1 -> scheme (Isrc/Upc/Ismn/Ean; ind1=7 -> $2 doi/isni/...)
  ignored; all 024 flattened to `bf:Identifier`. Reverse hardcodes ind1='8'. **[064]**
- GAP (med/high) -- no forward 010 -> `bf:Lccn` producer at all (reverse maps
  Lccn->010, so nothing round-trips in). **[064]**
- RESOLVED [065] -- 050/082/084 $b now split into `bf:itemPortion` (was merged
  into the portion); 082 carries its Dewey $2 as `bf:source` and its edition
  (ind1 0/1 -> full/abridged) as `bf:edition`. Reverse restores $a/$b/$2 and the
  082 edition indicator. `joinSub` retired.
- DIVERGENCE (med), kept [065] -- 037 stays flat (`bf:Identifier` with $b as
  `bf:source`); m2b builds `bf:acquisitionSource -> bf:AcquisitionSource`
  (StockNumber + agent). Modeling the acquisition-source node (new class + reverse)
  is deferred: the flat form preserves the number and agency and round-trips; the
  only loss is the agency being labeled a scheme, which is cosmetic here.
- DIVERGENCE (med), kept [065] -- 072 stays a source-qualified `bf:Classification`;
  m2b routes it to a `bf:subject`/`bf:Topic` category with `bf:code`. Moving it to
  the subject side would need subject-path plumbing (and a `bf:code` on Topic) for a
  category code that our flat, `--`-joined subject model does not otherwise carry;
  the classification form already preserves the $a code and $2 scheme and
  round-trips. Left as a deliberate divergence.
- GAP (low), deferred [065] -- 050 ind1/ind2 -> `bf:assigner`/`bf:status`, 082
  ind2/$q assigner, 084 $q, 020 $c price, 022 $l/$m, and bf:source as a
  dereferenceable IRI (vs the current label) remain unhandled; all low-frequency.

## 5. Provision / Physical / Language / Leader typing

Summary: coarse shape matches (one provision node + extent/media/carrier on
Instance, language + content class on Work) but consistently simplified: every
26X -> `bf:Publication`, RDA vocab IRIs stripped, no `bf:content`, no `bflc:simple*`.

- RESOLVED [066] -- one provision node per 26X, typed by 264 ind2 (0/1/2/3 ->
  Production/Publication/Distribution/Manufacture, 260/blank -> Publication);
  `Instance.Provisions []Provision`. Emitted as a `bf:provisionActivity` list so
  multiple nodes serialize as a JSON-LD array rather than duplicate keys.
- RESOLVED [066] -- 264 _4 -> `Instance.CopyrightDate` (`bf:copyrightDate`), not a
  provision node; the reverse emits it back as 264 _4 $c.
- RESOLVED [066] -- transcribed $a/$b go to `bflc:simplePlace`/`simpleAgent` (not a
  duplicate controlled label) and the date to `bf:date` + `bflc:simpleDate`.
- RESOLVED [066] -- 008/15-17 country is a controlled `bf:place` IRI in the LoC
  countries vocabulary on a Publication node (minted when the record has no usable
  26X); the reverse reconstructs a minimal 008 carrying just the country so it
  round-trips. Still no EDTF datatype on dates (deferred, cosmetic).
- RESOLVED [067] -- 336 $b -> `Work.Content` -> `bf:content` IRI in the RDA
  contentTypes vocabulary, with a leader/06 fallback (`content06`) so every Work
  carries a content term. Reverse emits 336 $b + $2 rdacontent.
- RESOLVED [067] -- 337/338 -> `[]RDATerm` (repeatable); a $b code drives the RDA
  `mediaTypes`/`carriers` IRI (label from $a), a $a-only term stays a blank labeled
  node. Reverse restores $a/$b/$2.
- RESOLVED [067] -- 300 extent is $a(+$b/$f/$g); $c is routed to `bf:dimensions`
  (round-tripped to 300 $c) rather than inflating the extent. $e still dropped
  (deferred, low-frequency).
- RESOLVED [068] -- `bf:Language` now carries `bf:code` (the three-letter code) and
  the vocabulary IRI, never `rdfs:label`=code. Reverse `langCode` still reads
  bf:code / IRI / (legacy) label, so LoC input keeps decoding.
- RESOLVED [068] -- 041 $h (language of the original) -> `Work.OriginalLangs`,
  emitted as a `bf:Language` with `bf:part` "original" and reversed back to 041 $h.
  041 $b (summary language) still unhandled (deferred, low-frequency); full
  bf:accompaniedBy related work remains out of scope (Hub model).
- MATCH -- leader/06 -> Work content class (Text/NotatedMusic/...).
- RESOLVED [070] -- leader/06 i/j now split into `bf:NonMusicAudio`/`bf:MusicAudio`
  (was a single `Audio`), inverted by `recordType` (Audio still maps to 'i' for
  external input). leader/07 -> `Instance.Issuance` -> `bf:issuance` IRI in the LoC
  issuance vocabulary (mono/serl/intg/coll), reversed to leader/07 by `leaderFor`.
- DIVERGENCE, kept [070] -- the Work is NOT given a second rdf:type from leader/07
  (bf:Monograph/Serial are non-standard and would collide with the single-subclass
  reverse `typeExcept`); the standard `bf:issuance` carries the signal instead. q ->
  Hub and a secondary `bf:Manuscript` type remain out of scope (Hub model / reverse
  cost). The issuance IRI is the round-trippable mode-of-issuance signal.

## 6. AdminMetadata / Notes / Locator / Linking

Summary: implemented fields match node shape (520 summary, 001 Local, 005
changeDate, 856 electronicLocator-on-Instance). The common 5xx notes [072] and the
common 76x-78x linking entries [073] now crosswalk; the long tail of each family
remains a tracked checklist in its task file.

- MATCH -- 520 -> `bf:summary`; 001 -> `bf:identifiedBy`/`bf:Local`; 005 ->
  `bf:changeDate`; 856 $u -> `bf:electronicLocator` on Instance.
- RESOLVED [072] -- common 5xx notes land as `bf:Note` with a round-trippable
  `bf:noteType`: 500 -> untyped note (Instance), 504 -> `bibliography` (Instance),
  546 -> `language` (Work), 505 -> `bf:tableOfContents` (Work). `Work.Notes`,
  `Instance.Notes` (`[]Note{Type,Label}`) and `Work.TableOfContents` model the split;
  repeated notes and ToC entries serialize as JSON-LD arrays (new `litList` sink)
  so they cannot collide on one object key. The long tail (508/511/521/524/525/
  533/534/502/506/540/538, 520 ind1 nuance) stays a tracked checklist in
  `tasks/072_bibframe_5xx_notes.done.md`.
- RESOLVED [073] -- the common 76x-78x linking entries land as
  `bf:relation -> bf:Relation`: a `bf:relationship` vocabulary IRI plus a
  `bf:associatedResource -> bf:Work` carrying the linked resource's title, optional
  creator and ISSN. 780/785 refine the relationship by second indicator (continues/
  supersedes/absorbed/... and their succeeding inverses); 773 -> `partOf`, 776 ->
  `otherPhysicalFormat`. `Work.Relations` and the `linkRelations` table drive both
  directions; the associated Work is a flat blank node (no minted IRI, a deliberate
  divergence -- see the note below), skipped as a standalone record on decode. The
  remaining tags (760/762/765/767/770/772/774/775/777/786/787, 490/8xx series ->
  `bf:hasSeries`) stay a tracked checklist in
  `tasks/073_bibframe_linking_entries.done.md`.
- DIVERGENCE (structural) -- m2b mints per-field Work/Instance IRIs
  (`#Work760-1`-style) for each linked resource; this crosswalk keeps the flat model
  and emits the `bf:associatedResource` as a blank labeled `bf:Work`, consistent with
  the name-title `bf:relatedTo` handling from 062.
- RESOLVED [082] -- 006/007 coded fields fold into the RDA media/carrier model
  through one bidirectional table (`carrier007`): a 007 in the sound, computer or
  video categories contributes its correlated carrier term on read (explicit
  337/338 win; coded fields only add what is missing), and decode rebuilds a
  minimal 2-byte 007 per mapped carrier plus the 006 'm' electronic aspect when a
  computer media type rides on a non-computer-file leader -- the same
  derive-don't-fabricate shape as the partial 008 reconstruction. Unmapped
  categories (maps, globes, microforms, ...) remain enumerated in
  `tasks/082_bibframe_006_007_coded_fields.done.md`.
- RESOLVED [081] -- downstream-driven round-trip batch (filed from libcat's
  fidelity gate): 511/521 -> typed Work notes (`performers`/`audience`), 533/538 ->
  typed Instance notes (`reproduction`/`systemDetails`), with note labels now
  joining every subfield (a multi-subfield 533 keeps its details); 490 $a ->
  `bf:seriesStatement` on the Instance (see [102] for $v); 776 `$z` -> a
  `bf:Isbn` on the associated resource (the OverDrive
  print/ebook pairing shape, previously dropped); 306 -> `bf:duration`; 347 $a/$b
  -> `bf:digitalCharacteristic` -> `bflc:FileType`/`bflc:EncodingFormat`. Repeated
  `bf:relatedTo`/`bf:relation` children now serialize as JSON-LD arrays -- the
  third instance of the duplicate-object-key class, caught by the new loss-gate
  matrix (`lossgate_test.go`), which round-trips a fully populated record and the
  realdata corpus through all four serializations and pins every tag as
  kept/transformed/lost.
- RESOLVED [069] -- 003 is read into `AdminMetadata.ControlOrg` and attached to the
  001 `bf:Local` as a `bf:assigner` agent (organizations-vocabulary IRI when the
  code is IRI-safe, plus `bf:code`). Only emitted when 003 is present -- no DLC
  default, to avoid falsely attributing non-LoC records.
- RESOLVED [069] -- every 040 $e -> a `bf:DescriptionConventions` node (RDA
  descriptionConventions IRI + `bf:code`), replacing the first-$e plain literal.
- RESOLVED [069] -- 005 `bf:changeDate` is now an `xsd:dateTime` typed literal via a
  new `litTyped` sink method (the crosswalk's first typed literal; both parsers
  already read datatypes). AdminMetadata stays forward-only provenance (not reversed
  to MARC, excluded from the round-trip by `normalize`), so this changed no goldens
  and needs no reverse.
- RESOLVED [094] -- 040 $b -> `bf:descriptionLanguage` (a `bf:Language` on the
  languages vocabulary), matching m2b's live template.
- GAP (low), deferred [069] -- 042 (`bf:descriptionAuthentication`) still unhandled.
- DIVERGENCE (low) -- 856 ind2 not consulted (ind2=2 -> `bf:supplementaryContent`,
  ToC -> `bf:tableOfContents`); no `bf:Item`/secondary Electronic Instance. **[072]**
- RESOLVED [102] -- 490 $v -> `bf:seriesEnumeration` (m2b's predicate for it,
  ConvSpec-Process6-Series.xsl), a literal on the Instance beside
  `bf:seriesStatement`. It was previously packed into the statement after an ISBD
  " ; " and split back on decode, which silently corrupted a series title that
  itself contained " ; " (`"Aims ; and methods"` decoded to $a "Aims" / $v "and
  methods"). A repeated $v keeps the first.
- DIVERGENCE (low), deliberate [102] -- m2b routes 490 through a grouped series
  entity (`bf:title`/`bf:Title` per group, with `groupNum` pairing the
  enumeration); we keep the flat `bf:seriesStatement` literal on the Instance,
  which is what downstream libcat's editor reads. Because flat sibling literals
  cannot say which statement an enumeration belongs to, we emit one
  `bf:seriesEnumeration` per statement in the same order -- including an empty
  literal for a 490 with no $v -- so position pairs them. Decode pairs by position
  when the counts match, pairs a lone statement with a lone enumeration (the shape
  a third-party graph writes), and otherwise drops the enumerations rather than
  attributing them to the wrong series.
- SUPERSET [094] -- 040 $a -> `bf:assigner` and $d -> `bf:descriptionModifier` are
  commented out in current m2b; we emit both, in m2b's own commented-out node shape
  (organizations IRI + `bf:code`). Two deliberate divergences: we emit one
  `bf:descriptionModifier` per $d, where m2b's dead code keeps only the last, and
  we emit `bf:assigner` for $a even when the agency is not DLC (m2b's dead code
  mints an IRI only for DLC).
- MATCH [094] -- 040 is also preserved whole as a `bf:Note` typed
  `mnotetype/internal` whose `rdfs:label` is the field in marcKey form, exactly as
  m2b does. This is the only carrier for 040 $c (a transcribing agency has no
  BIBFRAME property, and m2b has no template for it), so it is what makes 040
  round-trip field-exact. Decode prefers the note and falls back to the modelled
  properties for a graph that lacks one.

---

## Gap register (prioritized backlog)

Tier 1 -- high value, low/medium effort, preserves the flat model:

| Task | Area | Gap |
|------|------|-----|
| 060 | Subjects | `bf:source` from ind2/$2 + subdivision reverse fidelity; subdivided 655 |
| 061 | Contributions | $4 relator IRIs, role-as-IRI, multi-role, ind1 Family/Jurisdiction, $0/$1 |
| 062 | Contributions | 7xx $t name-title -> related work (fix misclassification) |
| 063 | Identifiers | `bf:status` for canceled/invalid ($z/$y on 020/022/024) |
| 064 | Identifiers | 024 ind1 -> scheme typing (fwd+rev); forward 010 -> `bf:Lccn` |

Tier 2 -- medium value/effort:

| Task | Area | Gap |
|------|------|-----|
| 065 | Classification | 050 itemPortion/assigner/status, 082/084 $b/$2/edition, 072-as-subject, 037 shape |
| 066 | Provision | 264 ind2 subclass + copyright + 008 country place + `bflc:simple*` |
| 067 | Physical/RDA | 336 content + leader/06 fallback; 337/338 RDA IRIs; 300 extent split |
| 068 | Language | `bf:code`/`bf:part` shape; drop `rdfs:label`=code; 041 $b/$h |
| 069 | AdminMetadata | 003 assigner, 040 $e node + all $e, 005 `xsd:dateTime`, 042 |
| 094 | AdminMetadata | 040 $a/$b/$c/$d agency chain + internal note; 040 regenerated on decode |
| 070 | Leader typing | leader/07 issuance + Monograph/Serial; i/j audio subclasses; q Hub |
| 071 | Titles | 245 nonSortNum; uniform $n/$p; 246 variant titles |

Tier 3 -- high effort or lower frequency:

| Task | Area | Gap |
|------|------|-----|
| 072 | Notes | 5xx note family -> `bf:Note` (Notes on Work/Instance); 856 ind2 |
| 073 | Linking | 76x-78x -> `bf:relation` + relationship-vocab IRI + `bf:associatedResource` |

Every gap task must keep goldens byte-identical unless it deliberately adds output
for a field the sample carries, and must add a round-trip/golden test for the new
signal. None of them require adopting the Hub/MADS structure; that remains an
explicit non-goal of this library's model.
