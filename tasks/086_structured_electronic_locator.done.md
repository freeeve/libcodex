# bibframe: carry 856 link labels, not just $u

## Gap

The 856 crosswalk (bibframe/bibframe.go, case "856") keeps only `$u` into
`Instance.ElectronicLocator []string`. Real vendor 856s carry display
context that is currently dropped:

```
856 40 $u http://link.overdrive.com/?...      $z Click to access digital title.
856 4  $3 Image     $u https://img1.od-cdn.com/ImageType-100/...Img100.jpg $z Large cover image
856 4  $3 Thumbnail $u https://img1.od-cdn.com/ImageType-200/...Img200.jpg $z Thumbnail cover image
856 4  $3 Excerpt   $u https://samples.overdrive.com/?...                  $z Sample
```

(every OverDrive MARC Express record; see libcatalog
`ingest/overdrive/testdata/marc-express/od-sample-ebook.mrc`.)

Consumers (libcatalog's editor links field) can only show the bare URL, or
guess labels from URL shapes -- libcatalog currently ships a client-side
heuristic (`backend/ui/src/lib/links.ts`) that should become unnecessary.

## Suggested shape

`ElectronicLocator` becomes a struct slice (breaking, or add a parallel
field):

```go
type ElectronicLocator struct {
    URL       string // $u
    Materials string // $3 (e.g. "Image", "Thumbnail", "Excerpt")
    Note      string // $z public note
    LinkText  string // $y
    // ind1/ind2 access-method/relationship could ride along as well
}
```

BIBFRAME modeling: bf:electronicLocator with rdfs:label (or bf:note) on a
locator node instead of a bare IRI object; the reverse crosswalk
(reader_crosswalk.go) should re-emit $3/$z/$y so 856 stays a CoreField
round-trip.

## Context

Filed from libcatalog task 090 (856 link labels + cover thumbnails in the
editor). Once available, libcatalog can store labeled locator nodes and
drop the URL-shape heuristic.

## Done (2026-07-06)

`ElectronicLocator []string` became `[]ElectronicLocator`
{URL, Materials, Note, LinkText} -- a breaking change, taken in 0.x.

Graph shape (chosen: distinct channels, full round-trip):
- Each 856 is an rdf:Description node (the $u URL as its IRI) hanging off
  bf:electronicLocator. $3 materials -> rdfs:label, $z -> a literal bf:note,
  $y -> a bf:note node typed `bf:noteType "link text"` so all four subfields
  survive Encode -> Decode. A URL-only 856 is an empty rdf:Description
  (same single triple as the old bare ref).
- Untyped-node support added to all three sinks (shape_render.go): graph skips
  the type triple, RDF/XML emits <rdf:Description>, JSON-LD emits an empty
  @type array -- the three stay isomorphic (verified by the existing 4-way
  serialization tests).
- Fixed a real bug surfaced by this change: xmlSink.beginNode wrote rdf:about
  unescaped (it had only ever seen safe internal IRIs); a locator URL is
  untrusted record data, so it now escapes via appendXMLAttr like ref() does.

Reverse crosswalk (reader_crosswalk.go locatorField) re-emits $u/$3/$z/$y.
Verified end-to-end on a real OverDrive export: $3 Image/Thumbnail/Excerpt
land as rdfs:label, $z as bf:note, and `&` in URLs escapes correctly.

Tests: locator_test.go (FromRecord parse, full four-subfield round-trip,
URL-only). libcatalog can now read rdfs:label off each locator node and drop
`backend/ui/src/lib/links.ts`.
