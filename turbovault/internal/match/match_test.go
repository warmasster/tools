package match

import "testing"

func TestMatches(t *testing.T) {
	cases := []struct {
		entry, page string
		want        bool
	}{
		{"https://github.com", "https://github.com/login", true},
		{"github.com", "https://github.com/login", true},
		{"https://www.github.com", "https://github.com", true},
		{"https://google.com", "https://accounts.google.com/signin", false},
		{"https://*.google.com", "https://accounts.google.com/signin", true},
		{"*.google.com", "https://google.com", true},
		{"https://*.com", "https://google.com", false},
		{"https://*.google.com", "https://google.com.evil.net", false},
		{"https://www.tumblr.com/login", "https://evil.tumblr.com/", false},
		{"https://example.com", "https://example.com:8443/", false},
		{"https://example.com:443", "https://example.com/", true},
		{"http://example.com", "https://example.com/", true},
		{"https://accounts.google.com", "https://mail.google.com", false},
		{"https://google.com", "https://google.com.evil.net", false},
		{"https://google.com", "https://evilgoogle.com", false},
		{"https://google.com", "https://notgoogle.com", false},
		{"https://bank.com", "http://bank.com", false},
		{"http://router.local", "http://router.local/admin", true},
		{"https://192.168.1.1", "https://192.168.1.1/", true},
		{"https://192.168.1.1", "https://192.168.1.10/", false},
		{"https://app.test:8443", "https://app.test:9000", false},
		{"https://app.test:8443", "https://app.test:8443/x", true},
		{"http://localhost:3000", "http://localhost:3000", true},
		{"https://com", "https://anything.com", false},
		{"android://abc@com.app", "https://com.app", false},
		{"https://github.com", "javascript:alert(1)", false},
		{"https://github.com", "file:///C:/x.html", false},
		{"https://GitHub.COM", "https://github.com", true},
	}
	for _, c := range cases {
		if got := Matches(c.entry, c.page); got != c.want {
			t.Errorf("Matches(%q, %q) = %v", c.entry, c.page, got)
		}
	}
	if Origin("https://www.Example.com:8080/x?y") != "https://example.com:8080" {
		t.Error(Origin("https://www.Example.com:8080/x?y"))
	}
	if Host("*.google.com") != "google.com" {
		t.Error(Host("*.google.com"))
	}
	if Score("*.google.com", "https://google.com") <= Score("*.google.com", "https://a.google.com") {
		t.Error("lo exacto debe puntuar más")
	}
}
