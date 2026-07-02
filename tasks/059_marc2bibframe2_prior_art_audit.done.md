# 059 -- bibframe: audit the crosswalk against LC's marc2bibframe2 prior art

Verify the BIBFRAME crosswalk (and, where it informs the MARC model, the codex
layer) follows LC's official converter rather than diverging silently. Recent
field additions (037/084/650_7 in 057, ISBN bf:qualifier in 058) were each checked
against marc2bibframe2 one field at a time; this task does the same sweep across
the whole crosswalk so we know where we match, where we deliberately differ, and
where we are simply incomplete.

This is an **audit**: the deliverable is a findings list, each item either
"matches", "deliberate divergence (why)", or a new numbered follow-up task for a
gap worth closing. Do not rewrite the crosswalk under this task.

## Reference sources

LC marc2bibframe2 (XSLT) and the BIBFRAME 2 ontology. WebFetch on id.loc.gov and
raw.githubusercontent.com returned 403/404 in this environment; fetch via the gh
API instead (this worked for 057/058):

    gh api repos/lcnetdev/marc2bibframe2/contents/xsl/<file>.xsl --jq '.content' | base64 -d
    gh api repos/lcnetdev/bibframe-ontology/contents/bibframe.rdf --jq '.content' | base64 -d

Key XSLT modules: `ConvSpec-0XX-*.xsl` (control/number/coded fields, identifiers,
classification), `ConvSpec-1XX-7XX-*.xsl` (names/contributions), `ConvSpec-24X`
(titles), `ConvSpec-25X-28X` (edition/provision), `ConvSpec-3XX` (physical/RDA),
`ConvSpec-5XX` (notes), `ConvSpec-6XX` (subjects), `ConvSpec-76X-78X` (linking),
`ConvSpec-8XX` (holdings/locator).

## Areas to audit (our impl -> prior art)

Cross-check each against the matching ConvSpec module; note Work-vs-Instance
placement, node types, predicate choice, and source/status handling.

- [ ] Leader/008 -> Work class and Instance/issuance typing (`workClass`, date008).
      Do we set bf:content / bf:issuance / carrier where m2b does?
- [ ] Titles: 245/240/130 -> bf:Title/bf:mainTitle/subtitle/part; nonfiling
      indicator handling; uniform vs transcribed; variant titles (246).
- [ ] Contributions: 100/110/111/700/710/711 -> bf:Contribution, bf:agent typing
      (Person/Organization/Meeting), relators from $4/$e, bflc:PrimaryContribution
      for the 1xx. Confirm role IRI vs literal matches m2b.
- [ ] Subjects: 6xx -> bf:subject; complex subjects with $x/$y/$z/$v. We currently
      join subdivisions with "--" into one label (`subdivided`); m2b builds a
      bf:ComplexSubject with component parts and a bf:source (LCSH etc.). Decide:
      match the component model or record the divergence.
- [ ] Classification: 050/082/072/084 -> bf:ClassificationLcc/Ddc/Classification;
      edition number, assigning source, and the $2 scheme.
- [ ] Identifiers: 020/022/024/010/037 -> bf:Isbn/Issn/Identifier/Lccn/Local.
      Qualifier done (058). Check bf:status (canceled/invalid $z, incorrect ISSN),
      and 024 ind1 -> scheme (isbn/doi/etc.) vs our fixed ind1='8' on reverse.
- [ ] Provision: 260/264 -> bf:Publication/Distribution/Manufacture/Production by
      264 ind2; bflc:simplePlace/simpleAgent/simpleDate transcription. Verify ind2
      mapping and that 260 falls back correctly.
- [ ] Physical/RDA: 300/336/337/338 -> extent/content/media/carrier. We read
      337/338 (media/carrier) but not 336 (content type) -- confirm whether m2b
      puts content on the Work as bf:content.
- [ ] Language: 008/41 -> bf:Language; $a/$b/$h roles; the language-code table.
- [ ] Notes: 5xx -> bf:Note subtypes (we currently take 520 summary only).
- [ ] Locator/holdings: 856 -> bf:electronicLocator on Instance vs bf:Item.
- [ ] AdminMetadata: 001/003/005/040 -> bf:AdminMetadata, generationProcess,
      descriptionConventions, bf:source; the record-control-number shape.
- [ ] Linking entries 76x-78x -> bf:relatedTo family (precededBy/succeededBy/...).
      Currently unhandled -- likely a gap to file separately.

## MARC-model spot check (secondary)

Where the crosswalk leans on codex behavior, confirm it against MARC 21:
indicator semantics we depend on (245 nonfiling, 264 ind2, 024 ind1), ISBD
punctuation trimming (`trimISBD`), and control-field 006/007/008 slicing.

## Acceptance

- [x] A findings note (in this file or a `docs/` audit note) covering each area
      above, classified matches / deliberate-divergence / gap.
- [x] Each actionable gap filed as its own numbered task with the m2b reference.
- [x] No behavior change committed under this task; goldens untouched.

Origin: user request after the 057/058 field-by-field prior-art checks -- do the
sweep once rather than per field.

## Result

Findings note: `docs/bibframe_m2b_audit.md` -- six subsystems (titles,
contributions, subjects, identifiers/classification, provision/physical/language/
leader, admin/notes/locator/linking) each compared against the matching
`ConvSpec-*.xsl` module and classified match / deliberate-divergence / gap, with a
prioritized gap register at the end.

Headline: the crosswalk matches m2b's node shapes for the fields it implements and
its simplifications (flat subject labels, no Hub, no MADS ComplexSubject) are
deliberate and preserved. The gaps are (a) dropped secondary signals -- subject
`bf:source`, identifier `bf:status`, 024 ind1 scheme, RDA content/media/carrier
IRIs, relator IRIs -- and (b) unimplemented breadth: the 5xx note family beyond
520 and the entire 76x-78x linking family.

Gaps filed as tasks **060-073** (Tier 1: 060-064; Tier 2: 065-071; Tier 3:
072-073). No code changed under this task.
