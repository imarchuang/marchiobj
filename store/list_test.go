package store

import (
	"bytes"
	"reflect"
	"testing"
)

func TestListPrefixAndDelimiter(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBucket("pics"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"photos/a.jpg", "photos/b.jpg", "docs/x"} {
		if _, err := st.PutObject("pics", k, bytes.NewReader([]byte(k))); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := st.ListObjects("pics", "photos/", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Objects) != 2 {
		t.Fatalf("prefix photos/: %+v", listed.Objects)
	}
	if listed.Objects[0].Key != "photos/a.jpg" || listed.Objects[1].Key != "photos/b.jpg" {
		t.Fatalf("keys %+v", listed.Objects)
	}

	rooted, err := st.ListObjects("pics", "", "/", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rooted.Objects) != 0 {
		t.Fatalf("unexpected objects %+v", rooted.Objects)
	}
	want := []string{"docs/", "photos/"}
	if !reflect.DeepEqual(rooted.CommonPrefixes, want) {
		t.Fatalf("common prefixes %+v", rooted.CommonPrefixes)
	}
}
