package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"testing"
)

func testTarget() Target {
	return Target{Resource: "ext", Host: "pg.example", Port: 5432, Database: "sag_ext", User: "reader"}
}

func testObjects() Objects {
	return Objects{
		Extensions: []Extension{
			{Name: "file_fdw", Version: "1.0", Schema: "public"},
			{Name: "dblink", Version: "1.2", Schema: "ext"},
		},
		Languages: []string{"plpython3u", "plperlu"},
	}
}

func TestFingerprintCanonical(t *testing.T) {
	doc := `{"resource":"ext","host":"pg.example","port":5432,"database":"sag_ext","user":"reader",` +
		`"extensions":[{"name":"dblink","version":"1.2","schema":"ext"},{"name":"file_fdw","version":"1.0","schema":"public"}],` +
		`"languages":["plperlu","plpython3u"]}`
	sum := sha256.Sum256([]byte(doc))
	want := hex.EncodeToString(sum[:])

	got, err := Fingerprint(testTarget(), testObjects())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if got != want {
		t.Fatalf("Fingerprint = %s; want %s (SHA-256 of %s)", got, want, doc)
	}
}

func TestFingerprintEmptyObjects(t *testing.T) {
	doc := `{"resource":"ext","host":"pg.example","port":5432,"database":"sag_ext","user":"reader","extensions":[],"languages":[]}`
	sum := sha256.Sum256([]byte(doc))
	want := hex.EncodeToString(sum[:])
	for _, o := range []Objects{{}, {Extensions: []Extension{}, Languages: []string{}}} {
		got, err := Fingerprint(testTarget(), o)
		if err != nil {
			t.Fatalf("Fingerprint: %v", err)
		}
		if got != want {
			t.Fatalf("Fingerprint(%+v) = %s; want %s", o, got, want)
		}
	}
}

func TestFingerprintChangesWithEachField(t *testing.T) {
	base, err := Fingerprint(testTarget(), testObjects())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	changes := map[string]func(*Target, *Objects){
		"resource":          func(t *Target, o *Objects) { t.Resource = "ext2" },
		"host":              func(t *Target, o *Objects) { t.Host = "pg2.example" },
		"port":              func(t *Target, o *Objects) { t.Port = 5433 },
		"database":          func(t *Target, o *Objects) { t.Database = "other" },
		"user":              func(t *Target, o *Objects) { t.User = "postgres" },
		"extension version": func(t *Target, o *Objects) { o.Extensions[0].Version = "1.1" },
		"extension schema":  func(t *Target, o *Objects) { o.Extensions[0].Schema = "x" },
		"new extension": func(t *Target, o *Objects) {
			o.Extensions = append(o.Extensions, Extension{Name: "adminpack", Version: "2.1", Schema: "pg_catalog"})
		},
		"removed language": func(t *Target, o *Objects) { o.Languages = o.Languages[:1] },
	}
	for name, change := range changes {
		target, objects := testTarget(), testObjects()
		change(&target, &objects)
		got, err := Fingerprint(target, objects)
		if err != nil {
			t.Fatalf("%s: Fingerprint: %v", name, err)
		}
		if got == base {
			t.Fatalf("%s: fingerprint did not change", name)
		}
	}
}

func TestSortedLists(t *testing.T) {
	want, err := Fingerprint(testTarget(), testObjects())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	o := testObjects()
	slices.Reverse(o.Extensions)
	slices.Reverse(o.Languages)
	extensions := slices.Clone(o.Extensions)
	languages := slices.Clone(o.Languages)
	got, err := Fingerprint(testTarget(), o)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if got != want {
		t.Fatalf("Fingerprint of reversed lists = %s; want %s", got, want)
	}
	if !slices.Equal(o.Extensions, extensions) || !slices.Equal(o.Languages, languages) {
		t.Fatal("Fingerprint changed the order of the input lists")
	}
}

func TestSortedListsSameNameByVersionAndSchema(t *testing.T) {
	a := Objects{Extensions: []Extension{{Name: "x", Version: "2", Schema: "b"}, {Name: "x", Version: "1", Schema: "a"}, {Name: "x", Version: "1", Schema: "b"}}}
	b := Objects{Extensions: []Extension{{Name: "x", Version: "1", Schema: "b"}, {Name: "x", Version: "2", Schema: "b"}, {Name: "x", Version: "1", Schema: "a"}}}
	fa, err := Fingerprint(testTarget(), a)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	fb, err := Fingerprint(testTarget(), b)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if fa != fb {
		t.Fatalf("Fingerprint depends on input order: %s != %s", fa, fb)
	}
}
