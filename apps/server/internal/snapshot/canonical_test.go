package snapshot

import "testing"

func TestCanonicalJSONNormalizesNumbersForJSONB(t *testing.T) {
	raw := []byte(`{
		"exponent": 1e2,
		"fraction": 1.2300,
		"negativeZero": -0,
		"shifted": 0.001e2,
		"largeInteger": 9007199254740993
	}`)

	canonical, err := CanonicalJSON(raw)
	if err != nil {
		t.Fatalf("canonicalize JSON: %v", err)
	}
	want := `{"exponent":100,"fraction":1.23,"largeInteger":9007199254740993,"negativeZero":0,"shifted":0.1}`
	if string(canonical) != want {
		t.Fatalf("unexpected canonical JSON\nwant: %s\n got: %s", want, canonical)
	}
}
