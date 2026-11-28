package secret

import (
	"testing"
)

func TestAWSNameRegexp(t *testing.T) {
	valid := []string{
		"a",
		"sag/test/conn",
		"sag/name/conn",
		"A123_+=.@-",
		"a/" + repeat("x", 509),
	}
	for _, name := range valid {
		if !awsNamePattern.MatchString(name) {
			t.Errorf("awsNamePattern rejected valid name %q", name)
		}
	}

	invalid := []string{
		"",
		repeat("x", 513),
		"name with space",
		"name!",
		"name#",
	}
	for _, name := range invalid {
		if awsNamePattern.MatchString(name) {
			t.Errorf("awsNamePattern accepted invalid name %q", name)
		}
	}
}

func TestSecretOnlyFields(t *testing.T) {
	cases := []struct {
		value string
	}{
		{"aws-sm:sag/test/conn"},
	}
	for _, c := range cases {
		_, err := ParseRef(c.value)
		if err != nil {
			t.Errorf("ParseRef(%q) unexpected error: %v", c.value, err)
		}
	}

	_, err := ParseRef("secret:valid-name")
	if err != nil {
		t.Fatalf("ParseRef(secret:valid-name) unexpected error: %v", err)
	}

	_, err = ParseRef("other:name")
	if err == nil {
		t.Error("ParseRef(other:name) must return error for unrecognized prefix")
	}

	r, err := ParseRef("aws-sm:sag/name")
	if err != nil {
		t.Fatalf("ParseRef(aws-sm:sag/name) unexpected error: %v", err)
	}
	if r.Backend != BackendAWSSM {
		t.Errorf("ParseRef aws-sm: got Backend=%q, want %q", r.Backend, BackendAWSSM)
	}
	if r.Name != "sag/name" {
		t.Errorf("ParseRef aws-sm: got Name=%q, want %q", r.Name, "sag/name")
	}
}

func repeat(s string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}

func TestParseRefRejectsBadAWSName(t *testing.T) {
	for _, s := range []string{"aws-sm:", "aws-sm:name with space", "aws-sm:name!", "aws-sm:" + repeat("x", 513)} {
		if _, err := ParseRef(s); err == nil {
			t.Errorf("ParseRef(%q) = nil error; want a name error", s)
		}
	}
	if ref, err := ParseRef("aws-sm:sag/a/conn"); err != nil || ref.Backend != BackendAWSSM || ref.Name != "sag/a/conn" {
		t.Errorf("ParseRef(aws-sm:sag/a/conn) = %+v, %v", ref, err)
	}
}
