# 061 -- bibframe: contribution relator IRIs and agent fidelity

Tier 1 (high value). From the 059 m2b audit, contributions area.
Ref: `docs/bibframe_m2b_audit.md` section 2; m2b `ConvSpec-1XX,7XX,8XX-names.xsl`.

## Motivation

Our contributions capture the shape (`bf:contribution` -> `bf:Contribution` +
`bf:agent`, 1xx primary) but lose the controlled-vocabulary content: the role is
always a bare `rdfs:label` and the agent is an anonymous node labeled from $a only.
m2b maps $4 to a `http://id.loc.gov/vocabulary/relators/<code>` IRI and types
agents by ind1.

## Scope

1. **$4 relator code -> relator IRI.** A 3-char $4 becomes the `bf:role` node's
   IRI (`.../vocabulary/relators/<code>`); a URI $4 is used verbatim; else fall back
   to an $e literal role. Prefer $4 over $e (reverse of today's order).
2. **Multi/compound roles.** Iterate all $e/$4; split a single $e on `, and &`.
3. **Agent ind1 typing.** x00 ind1=3 -> `bf:Family`, x10 ind1=1 -> `bf:Jurisdiction`
   (our reverse path already knows these classes; make forward symmetric).
4. **Agent label.** Concatenate the tag-appropriate subfield set (x00 $a$b$c$d$q$j$k,
   x10 $a$b$c$d$n$g$k, x11 $a$c$d$e$n$g$q) instead of $a only.
5. **Authority $0/$1** (optional, lower priority): mint/attach the agent IRI or an
   authority link from $1/$0 (id.loc.gov/authorities/names).
6. Minor: 111/711 relator from $j not $e.

## Hazards

- Sample 100 has $e "author"; today that yields a literal role. Emitting a
  relators IRI changes goldens -- regenerate deliberately. 700 has $4 "edt".
- Keep primary `bflc:PrimaryContribution` typing; don't regress it.
- Reverse path (`contributions`/`roleNode`) must read the relator IRI back to $4/$e.

## Acceptance

- [x] $4 -> relator IRI; role node carries the IRI, not a bare label.
- [x] Family/Jurisdiction typed from ind1; agent label uses the full subfield set.
- [x] Round-trip preserves role (as $4 when it was a code); goldens + fuzz green.

## Result

`Contribution.Role string` became `Contribution.Roles []Role`, each `Role` an
optional relator `IRI` plus an rdfs:label `Term`. `appendContribution` now:

- builds the agent label by concatenating the tag-appropriate name subfields
  (`agentLabel`: x00 a,b,c,d,q,j,k / x10 a,b,c,d,n,g,k / x11 a,c,d,e,n,g,q) instead
  of $a only;
- refines the class via `agentSubclass` from ind1 (x00 ind1=3 -> Family, x10
  ind1=1 -> Jurisdiction), which the reverse `contribTag`/`agentClasses` already
  understood;
- collects roles with `contribRoles`: every $4 first (`relatorRole` maps a 3-letter
  code to `.../vocabulary/relators/<code>`, an XML-safe absolute URI verbatim, else
  a literal), then the literal-role subfield ($e for names, $j for meetings) split
  on the ", and &" compound-role delimiters (`splitRoleTerms`).

`emitContribution` emits one `bf:role` node per Role (`emitRole`): an IRI-typed
`bf:Role` for a relator IRI, a blank `bf:Role` for a bare term. Reverse
(`roleSubfields`) turns an IRI role back into $4 (relator code when under the
relators vocab, else the whole IRI) and a literal role into $e/$j; `ind1ForClass`
restores the first indicator the forward path reads back to each class, which
matters now that ind1 selects Family/Jurisdiction.

Well-formedness: the XML sink writes node IRIs unescaped, so `isSafeIRI` gates a
URI-valued $4 (rejecting `<`, `>`, `&`, `"`, `'`, whitespace, controls); a
three-letter relator code is known-safe. An unsafe $4 falls back to a literal role.

Goldens: only the sample's 700 $4 "edt" role gained
`rdf:about=".../relators/edt"` (100 $e "author" stays a literal role; single-$a
labels are unchanged). Regenerated deliberately. Tests:
`contribution_relators_test.go` (relator IRI + $4-over-$e, compound roles,
Family/Jurisdiction typing + full label, meeting $j vs $4, verbatim/unsafe URI,
full round-trip). Suite + FuzzFromMARC + FuzzDecode green.

Not done (deferred, item 5): authority $0/$1 -> agent IRI/authority link -- lower
priority, left for a later pass.
