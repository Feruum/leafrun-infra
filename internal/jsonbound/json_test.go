package jsonbound

import "testing"

func TestBoundedUnambiguousJSON(t *testing.T) {
	for _, data := range [][]byte{[]byte(`{"a":1,"a":2}`), []byte(`{"a":{},"extra":{"b":1,"b":2}}`), []byte(`{"a":1} {}`), append([]byte(`{"a":"`), append([]byte{0xff}, []byte(`"}`)...)...)} {
		var out any
		if Decode(data, &out) == nil {
			t.Error("ambiguous JSON accepted")
		}
	}
	var out struct {
		A int `json:"a"`
	}
	if Decode([]byte(`{"a":1,"documented_extra":true}`), &out) != nil || out.A != 1 {
		t.Fatal("documented extra rejected")
	}
}
