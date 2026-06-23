package bibframe

import "testing"

// benchFormats returns the sample record serialized in each RDF format.
func benchFormats() (rdfxml, jsonld, nt, ttl []byte) {
	r := sample()
	rdfxml, _ = Encode(r)
	jsonld, _ = EncodeJSONLD(r)
	nt, _ = EncodeNTriples(r)
	ttl, _ = EncodeTurtle(r)
	return
}

func BenchmarkDecodeRDFXML(b *testing.B) {
	data, _, _, _ := benchFormats()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeJSONLD(b *testing.B) {
	_, data, _, _ := benchFormats()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeNTriples(b *testing.B) {
	_, _, data, _ := benchFormats()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeTurtle(b *testing.B) {
	_, _, _, data := benchFormats()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeNTriples(b *testing.B) {
	r := sample()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := EncodeNTriples(r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeTurtle(b *testing.B) {
	r := sample()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := EncodeTurtle(r); err != nil {
			b.Fatal(err)
		}
	}
}
