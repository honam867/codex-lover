//go:build !windows

package notify

import "testing"

func urgencyOf(args []string) string {
	for i, a := range args {
		if a == "-u" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestNotifySendArgsUrgency(t *testing.T) {
	cases := []struct {
		level Level
		want  string
	}{
		{LevelInfo, "low"},
		{LevelWarning, "normal"},
		{LevelError, "critical"},
		{Level(""), "normal"},
	}
	for _, tc := range cases {
		if got := urgencyOf(notifySendArgs(Event{Title: "T", Message: "M", Level: tc.level})); got != tc.want {
			t.Fatalf("level %q -> urgency %q, want %q", tc.level, got, tc.want)
		}
	}
}

func TestNotifySendArgsCarriesTitleAndBody(t *testing.T) {
	args := notifySendArgs(Event{Title: "Hello", Message: "World", Level: LevelInfo})
	if len(args) < 2 || args[len(args)-2] != "Hello" || args[len(args)-1] != "World" {
		t.Fatalf("expected title/body at end of args, got %#v", args)
	}
}
