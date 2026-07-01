# 049 -- unimarc: subject/name/008 mapping bugs, charset detection, hot path

## Motivation

Review found two high-severity semantic corruption bugs in the UNIMARC to
MARC 21 conversion (wrong data, not lost data -- worse, because nothing
flags it), a wrong 008 type-of-date, incomplete charset detection, and
several cheap hot-path fixes. unimarc also has the lowest coverage in the
repo (88.3%).

## Problems

1. **$y/$z subject subdivisions swapped** (high -- tomarc21.go:196-213).
   In UNIMARC 606/607, $y is *geographical* and $z is *chronological* --
   the reverse of MARC 21 6xx. `addSubject` copies y->y, z->z, so
   `606 $a Art $y France $z 19e siècle` becomes MARC data that mods then
   renders as `<temporal>France</temporal>` /
   `<geographic>19e siècle</geographic>`. Map y->z and z->y.
2. **Personal-name inversion loses the comma** (high -- accessors.go:33-46).
   UNIMARC 700 carries entry element in $a ("Dupont") and rest-of-name in
   $b ("Jean") with no stored punctuation; `personName` joins with a space,
   yielding `100 $a Dupont Jean`. Downstream: `citation.citeKey` derives
   the surname from text before the comma, and BibTeX consumers parse
   `Dupont Jean` as given-name "Dupont", surname "Jean" -- inverted. When
   Ind2 marks surname-first entry and $b lacks leading punctuation, join
   with ", ". Note unimarc_test.go:166 currently codifies the bad output
   ("Dupont Jean") -- update it.
3. **008/06 type-of-date copied verbatim across differing code tables**
   (tomarc21.go:94). UNIMARC 100$a/8 `d` = monograph complete when issued
   (MARC `s`), but MARC `d` = continuing resource ceased. The richRecord
   test's own 2020 monograph currently produces an 008 any MARC consumer
   reads as a dead serial. Add the translation table (a->c, b->d, d->s,
   f->q, g->m, h/i->t, j->e, u->n, default `s` when date1 present).
4. **Charset detection reads only 100$a/26-27 and mislabels code 02**
   (unimarc.go:30-33, :44-60). Code 01 is ISO 646, 02 is basic Cyrillic --
   ISO 5426 is code 03, conventionally in the second slot (28-29). The
   current "01"/"02" trigger works for the common "0103" pattern only by
   accident; genuinely Cyrillic records go through the Latin decoder and
   03/04/05 declarations fall through as "already UTF-8" -- silent
   mojibake. Read both slots; trigger 5426 on "03" in either; support or
   explicitly flag the Cyrillic/Greek codes (pairs with the lossiness
   signal from task 040).
5. **`forceUTF8Leader` copies every record** (unimarc.go:65-72, via Read at
   :120). The `raw` buffer in `Reader.Read` is freshly allocated and
   exclusively owned; mutate `raw[9]` in place there and keep the
   defensive copy only in the exported `Decode`.
6. **Per-field/per-call allocations** (tomarc21.go:108-135, :87-103;
   accessors.go:19-28, :82-92). `codeMap` builds a fresh map per retagged
   field (precompute the five constant maps or scan pairs); `build008`
   allocates two `strings.Repeat` per record (hoist), lines 100-101 are
   no-ops, and the `!= ""` guard at :16 is dead -- a record with no 100
   still gets a content-free 008 (return "" instead). `Authors`/`Subjects`
   call `r.DataFields` once per tag (8 full record scans + allocations);
   use a single pass with a tag switch.

## Acceptance

- [ ] Round-trip test with both $y and $z subdivisions produces correct
      MARC 21 and correct mods temporal/geographic output.
- [ ] `Dupont, Jean` restored; citation citeKey and BibTeX author tests
      cover the UNIMARC path.
- [ ] richRecord's 008/06 is `s`; table-driven test over the code list.
- [ ] "0103"-pattern and Cyrillic records detected correctly; unsupported
      sets flagged, not passed through silently.
- [ ] Decode-path allocations per record reduced (benchmark added --
      unimarc currently has none) and coverage raised toward the >=90%
      neighborhood of the sibling packages.
