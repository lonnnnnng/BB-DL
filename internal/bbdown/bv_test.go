package bbdown

import "testing"

func TestBVConverterKnownPairs(t *testing.T) {
	cases := []struct {
		aid  int64
		bvid string
	}{
		{aid: 626497566, bvid: "BV1qt4y1X7TW"},
		{aid: 48144058, bvid: "BV1Kb411W75N"},
		{aid: 116734744857333, bvid: "BV18dEr6DErw"},
	}

	for _, tc := range cases {
		encoded, err := EncodeBV(tc.aid)
		if err != nil {
			t.Fatalf("EncodeBV(%d) returned error: %v", tc.aid, err)
		}
		if encoded != tc.bvid {
			t.Fatalf("EncodeBV(%d) = %q, want %q", tc.aid, encoded, tc.bvid)
		}

		decoded, err := DecodeBV(tc.bvid)
		if err != nil {
			t.Fatalf("DecodeBV(%q) returned error: %v", tc.bvid, err)
		}
		if decoded != tc.aid {
			t.Fatalf("DecodeBV(%q) = %d, want %d", tc.bvid, decoded, tc.aid)
		}

		decodedSuffix, err := DecodeBV(tc.bvid[3:])
		if err != nil {
			t.Fatalf("DecodeBV(%q suffix) returned error: %v", tc.bvid, err)
		}
		if decodedSuffix != tc.aid {
			t.Fatalf("DecodeBV(%q suffix) = %d, want %d", tc.bvid, decodedSuffix, tc.aid)
		}
	}
}

func TestBVConverterRejectsOutOfRangeAid(t *testing.T) {
	if _, err := EncodeBV(0); err == nil {
		t.Fatal("EncodeBV(0) should reject aid smaller than 1")
	}
	if _, err := EncodeBV(maskCode + 1); err == nil {
		t.Fatal("EncodeBV(maskCode + 1) should reject aid outside BV range")
	}
}
