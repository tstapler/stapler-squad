package linear

import "testing"

func TestGetKeychainToken_should_ReturnEmpty_When_NothingStored(t *testing.T) {
	_ = DeleteKeychainToken()
	if got := GetKeychainToken(); got != "" {
		t.Fatalf("GetKeychainToken() = %q, want empty", got)
	}
}

func TestKeychainToken_should_RoundTripAndDelete(t *testing.T) {
	if err := SetKeychainToken("lin_api_abc"); err != nil {
		t.Fatal(err)
	}
	if got := GetKeychainToken(); got != "lin_api_abc" {
		t.Fatalf("GetKeychainToken() = %q, want lin_api_abc", got)
	}
	if err := DeleteKeychainToken(); err != nil {
		t.Fatal(err)
	}
	if got := GetKeychainToken(); got != "" {
		t.Fatalf("after delete GetKeychainToken() = %q, want empty", got)
	}
}
