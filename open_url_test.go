package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/deeplink"
)

func TestOpenURLSubcommand_should_ShellToOSOpener_When_ValidSsqURLGiven(t *testing.T) {
	origOpener := osOpenerFunc
	t.Cleanup(func() { osOpenerFunc = origOpener })

	var gotURL string
	called := false
	osOpenerFunc = func(_ context.Context, targetURL string) error {
		called = true
		gotURL = targetURL
		return nil
	}

	err := runOpenURL(context.Background(), "ssq://myhost/backlog/v1/bl_01J000")
	if err != nil {
		t.Fatalf("runOpenURL() error = %v, want nil", err)
	}
	if !called {
		t.Fatal("expected osOpenerFunc to be invoked, it was not")
	}
	want := "http://localhost:8543/backlog?item=bl_01J000"
	if gotURL != want {
		t.Errorf("opener called with URL = %q, want %q", gotURL, want)
	}
}

func TestOpenURLSubcommand_should_ExitNonZeroWithSingleStderrLine_When_URLMalformed(t *testing.T) {
	origOpener := osOpenerFunc
	t.Cleanup(func() { osOpenerFunc = origOpener })

	called := false
	osOpenerFunc = func(_ context.Context, _ string) error {
		called = true
		return nil
	}

	err := runOpenURL(context.Background(), "not-a-valid-url")
	if err == nil {
		t.Fatal("runOpenURL() error = nil, want non-nil for malformed input")
	}
	if !errors.Is(err, deeplink.ErrMalformed) {
		t.Errorf("runOpenURL() error = %v, want wrapping deeplink.ErrMalformed", err)
	}
	if called {
		t.Error("expected osOpenerFunc NOT to be invoked for malformed input")
	}
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		t.Errorf("error message should be a single line, got %q", msg)
	}
	if strings.Contains(strings.ToLower(msg), "goroutine") || strings.Contains(msg, "panic") {
		t.Errorf("error message should never look like a stack trace, got %q", msg)
	}
}

func Test_translateDeepLinkURL_should_BuildLocalhostItemURL_When_GivenValidDeepLink(t *testing.T) {
	got, err := translateDeepLinkURL("ssq://otherhost/backlog/v1/bl_01J999")
	if err != nil {
		t.Fatalf("translateDeepLinkURL() error = %v, want nil", err)
	}
	want := "http://localhost:8543/backlog?item=bl_01J999"
	if got != want {
		t.Errorf("translateDeepLinkURL() = %q, want %q", got, want)
	}
}

func Test_translateDeepLinkURL_should_EscapeQueryValue_When_IDContainsQueryMetacharacters(t *testing.T) {
	// A raw ID of "bl_01J999&evil=1" (as it would appear after url.Parse
	// decodes a percent-encoded '&' in the path segment) must not inject an
	// extra query parameter into the URL handed to the OS opener.
	got, err := translateDeepLinkURL("ssq://otherhost/backlog/v1/bl_01J999%26evil=1")
	if err != nil {
		t.Fatalf("translateDeepLinkURL() error = %v, want nil", err)
	}
	want := "http://localhost:8543/backlog?item=bl_01J999%26evil%3D1"
	if got != want {
		t.Errorf("translateDeepLinkURL() = %q, want %q (unescaped injection)", got, want)
	}
}

func Test_translateDeepLinkURL_should_ReturnError_When_GivenMalformedURL(t *testing.T) {
	_, err := translateDeepLinkURL("http://not-ssq-scheme/foo")
	if !errors.Is(err, deeplink.ErrMalformed) {
		t.Errorf("translateDeepLinkURL() error = %v, want wrapping deeplink.ErrMalformed", err)
	}
}
