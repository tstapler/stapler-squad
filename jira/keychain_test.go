package jira

import "testing"

func TestKeychainTokenForHost_should_RoundTrip(t *testing.T) {
	if err := SetKeychainTokenForHost("https://acme.atlassian.net", "alice@acme.com", "tok_abc"); err != nil {
		t.Fatal(err)
	}
	email, tok, ok := GetKeychainTokenForHost("https://acme.atlassian.net/")
	if !ok || email != "alice@acme.com" || tok != "tok_abc" {
		t.Fatalf("got (%q, %q, %v)", email, tok, ok)
	}
}

func TestKeychainTokenForHost_should_ReturnNotOK_When_HostMissing(t *testing.T) {
	if _, _, ok := GetKeychainTokenForHost("https://nobody.atlassian.net"); ok {
		t.Fatal("expected ok=false for unknown host")
	}
}

func TestKeychainTokenForHost_should_NotCollide_When_HostsDiffer(t *testing.T) {
	_ = SetKeychainTokenForHost("https://a.atlassian.net", "a@x.com", "ta")
	_ = SetKeychainTokenForHost("https://b.atlassian.net", "b@x.com", "tb")
	_, ta, _ := GetKeychainTokenForHost("https://a.atlassian.net")
	_, tb, _ := GetKeychainTokenForHost("https://b.atlassian.net")
	if ta != "ta" || tb != "tb" {
		t.Fatalf("collision: %q %q", ta, tb)
	}
	if err := DeleteKeychainTokenForHost("https://a.atlassian.net"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := GetKeychainTokenForHost("https://a.atlassian.net"); ok {
		t.Fatal("expected deleted")
	}
}
