package main

import "testing"

func TestCodexLoginURLPattern(t *testing.T) {
	line := "https://auth.openai.com/oauth/authorize?response_type=code&client_id=app_x&redirect_uri=http%3A%2F%2F127.0.0.1%3A1455%2Fauth%2Fcallback&state=abc"
	if got := codexLoginURLPattern.FindString("  " + line + "\r"); got != line {
		t.Fatalf("got %q", got)
	}
	if got := codexLoginURLPattern.FindString("Starting local login server on http://localhost:1455."); got != "" {
		t.Fatalf("local server line must not match, got %q", got)
	}
}
