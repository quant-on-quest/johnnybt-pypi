package pkgmeta

import "testing"

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"johnnybt":         "johnnybt",
		"JohnnyBT":         "johnnybt",
		"johnnybt_xbx":     "johnnybt-xbx",
		"johnnybt.xbx":     "johnnybt-xbx",
		"johnnybt-_.xbx":   "johnnybt-xbx",
		"Johnny__BT--Data": "johnny-bt-data",
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidName(t *testing.T) {
	for _, ok := range []string{"a", "johnnybt", "johnnybt_xbx", "a1.b-c_d"} {
		if !ValidName(ok) {
			t.Errorf("ValidName(%q) should be true", ok)
		}
	}
	for _, bad := range []string{"", "-a", "a-", "a b", "a/b", "中文"} {
		if ValidName(bad) {
			t.Errorf("ValidName(%q) should be false", bad)
		}
	}
}

func TestSafeFilename(t *testing.T) {
	for _, ok := range []string{"johnnybt_demo-0.1.0-py3-none-any.whl", "johnnybt_demo-0.1.0.tar.gz", "x-1.0+local.zip"} {
		if !SafeFilename(ok) {
			t.Errorf("SafeFilename(%q) should be true", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x.whl", "a/b.whl", "a b.whl", "a\\b.whl"} {
		if SafeFilename(bad) {
			t.Errorf("SafeFilename(%q) should be false", bad)
		}
	}
}

func TestParseFilename(t *testing.T) {
	cases := []struct {
		in, name, version string
		kind              Kind
		err               bool
	}{
		{"johnnybt_demo-0.1.0-py3-none-any.whl", "johnnybt_demo", "0.1.0", KindWheel, false},
		{"numpy-2.1.0-1-cp312-cp312-manylinux_2_17_x86_64.whl", "numpy", "2.1.0", KindWheel, false},
		{"johnnybt_demo-0.1.0.tar.gz", "johnnybt_demo", "0.1.0", KindSdist, false},
		{"johnnybt_demo-0.1.0.zip", "johnnybt_demo", "0.1.0", KindSdist, false},
		{"johnnybt_demo-0.1.0rc1-py3-none-any.whl", "johnnybt_demo", "0.1.0rc1", KindWheel, false},
		{"broken.whl", "", "", KindWheel, true},
		{"a-b-c-d-e-f-g.whl", "", "", KindWheel, true},
		{"nodash.tar.gz", "", "", KindSdist, true},
		{"readme.txt", "", "", KindUnknown, true},
	}
	for _, c := range cases {
		if got := KindOf(c.in); got != c.kind {
			t.Errorf("KindOf(%q) = %v, want %v", c.in, got, c.kind)
		}
		name, version, err := ParseFilename(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParseFilename(%q) err = %v, want err=%v", c.in, err, c.err)
			continue
		}
		if name != c.name || version != c.version {
			t.Errorf("ParseFilename(%q) = (%q, %q), want (%q, %q)", c.in, name, version, c.name, c.version)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.10.0", "1.9.0", 1},
		{"1.0.0rc1", "1.0.0", -1},
		{"1.0.0", "1.0.0.post1", -1},
		{"2.0.0.dev1", "1.9.9", 1},
		{"1.0", "1.0.0", 0},
		{"garbage", "1.0.0", -1}, // unparseable sorts first
		{"1.0.0", "garbage", 1},
		{"a", "b", -1}, // both unparseable: string order
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if !ValidVersion("1.2.3") || ValidVersion("not a version") {
		t.Error("ValidVersion misclassified")
	}
}
