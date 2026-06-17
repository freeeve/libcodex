#!/usr/bin/env python3
"""Interop helper for libcodex: reads the library's output with independent,
widely-used parsers and reports the result as one JSON line on stdout.

Driven by interop_test.go. Each command exits non-zero on failure. Run
`interop.py check` to verify the required libraries are importable.

Required: pymarc, rdflib, bibtexparser, rispy.
"""
import json
import sys


def emit(d):
    print(json.dumps(d))
    sys.exit(0 if d.get("ok") else 1)


def main():
    cmd = sys.argv[1]

    if cmd == "check":
        import bibtexparser  # noqa: F401
        import pymarc  # noqa: F401
        import rdflib  # noqa: F401
        import rispy  # noqa: F401
        emit({"ok": True})

    elif cmd == "marc-to-json":
        # Read binary ISO 2709 with pymarc, write it back out as MARC-in-JSON.
        import pymarc
        recs = list(pymarc.MARCReader(open(sys.argv[2], "rb"), to_unicode=True, force_utf8=True))
        json.dump([r.as_dict() for r in recs], open(sys.argv[3], "w"))
        emit({"ok": True, "records": len(recs)})

    elif cmd == "dump-fields":
        # Parse binary ISO 2709 with pymarc and dump each record's fields so the
        # Go side can compare its own parse field-by-field (a differential check).
        import pymarc
        recs = list(pymarc.MARCReader(open(sys.argv[2], "rb"), to_unicode=True, force_utf8=False))
        out = []
        for r in recs:
            fields = []
            for f in r.get_fields():
                if f.is_control_field():
                    fields.append({"tag": f.tag, "data": f.data})
                    continue
                subs = []
                for sf in f.subfields:
                    # pymarc >=5 yields Subfield(code, value); older yields a flat list.
                    if hasattr(sf, "code"):
                        subs.append([sf.code, sf.value])
                    else:
                        subs.append([sf, ""])
                fields.append({"tag": f.tag, "ind1": f.indicator1,
                               "ind2": f.indicator2, "subfields": subs})
            out.append({"leader": str(r.leader), "fields": fields})
        json.dump(out, open(sys.argv[3], "w"))
        emit({"ok": True, "records": len(recs)})

    elif cmd == "read-json":
        import pymarc
        recs = list(pymarc.JSONReader(open(sys.argv[2]).read()))
        emit({"ok": True, "records": len(recs), "title": recs[0].title if recs else ""})

    elif cmd == "parse-rdf":
        import rdflib
        g = rdflib.Graph().parse(sys.argv[2], format=sys.argv[3])
        emit({"ok": len(g) > 0, "triples": len(g)})

    elif cmd == "parse-bibtex":
        import bibtexparser
        text = open(sys.argv[2]).read()
        if hasattr(bibtexparser, "parse_string"):
            lib = bibtexparser.parse_string(text)
            entries, failed = lib.entries, len(lib.failed_blocks)
        else:
            db = bibtexparser.loads(text)
            entries, failed = db.entries, 0
        emit({"ok": failed == 0, "entries": len(entries), "failed": failed})

    elif cmd == "parse-ris":
        import rispy
        entries = rispy.loads(open(sys.argv[2]).read())
        emit({"ok": True, "entries": len(entries)})

    else:
        emit({"ok": False, "error": "unknown command %r" % cmd})


if __name__ == "__main__":
    try:
        main()
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": "%s: %s" % (type(e).__name__, e)}))
        sys.exit(1)
