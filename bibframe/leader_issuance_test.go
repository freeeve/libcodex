package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// leaderWith builds a leader string with byte 6 (type) and byte 7 (level) set.
func leaderWith(recType, level byte) codex.Leader {
	b := []byte("00000nam a2200000 a 4500")
	b[6] = recType
	b[7] = level
	return codex.Leader(b)
}

// TestAudioSubclasses covers leader/06 i/j splitting into NonMusicAudio/MusicAudio
// and round-tripping through the leader (task 070).
func TestAudioSubclasses(t *testing.T) {
	for _, tc := range []struct {
		recType byte
		class   string
	}{{'i', "NonMusicAudio"}, {'j', "MusicAudio"}} {
		rec := codex.NewRecord().
			SetLeader(leaderWith(tc.recType, 'm')).
			AddField(codex.NewControlField("001", "x")).
			AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
		if g := FromRecord(rec); g.Work.Class != tc.class {
			t.Errorf("leader/06 %c -> class %q, want %q", tc.recType, g.Work.Class, tc.class)
		}
		encoded, _ := Encode(rec)
		recs, err := Decode(encoded)
		if err != nil || len(recs) != 1 {
			t.Fatalf("Decode: %v (%d)", err, len(recs))
		}
		if rt := recs[0].Leader().RecordType(); rt != tc.recType {
			t.Errorf("%s leader/06 round-trip = %c, want %c", tc.class, rt, tc.recType)
		}
	}
}

// TestIssuance covers leader/07 -> Instance bf:issuance and the round-trip back to
// the leader's bibliographic level (task 070).
func TestIssuance(t *testing.T) {
	for _, tc := range []struct {
		level byte
		code  string
	}{{'m', "mono"}, {'s', "serl"}, {'i', "intg"}, {'c', "coll"}} {
		rec := codex.NewRecord().
			SetLeader(leaderWith('a', tc.level)).
			AddField(codex.NewControlField("001", "x")).
			AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
		if g := FromRecord(rec); g.Instance.Issuance != tc.code {
			t.Errorf("leader/07 %c -> issuance %q, want %q", tc.level, g.Instance.Issuance, tc.code)
		}
		encoded, _ := Encode(rec)
		recs, err := Decode(encoded)
		if err != nil || len(recs) != 1 {
			t.Fatalf("Decode: %v (%d)", err, len(recs))
		}
		if bl := recs[0].Leader().BibLevel(); bl != tc.level {
			t.Errorf("issuance %q leader/07 round-trip = %c, want %c", tc.code, bl, tc.level)
		}
	}
}
