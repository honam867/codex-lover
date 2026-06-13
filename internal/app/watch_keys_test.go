package app

import "testing"

func TestDecodeKeySingleBytes(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want watchKey
		n    int
	}{
		{"down j", []byte("j"), keyDown, 1},
		{"up k", []byte("k"), keyUp, 1},
		{"switch s", []byte("s"), keySwitch, 1},
		{"switch enter cr", []byte("\r"), keySwitch, 1},
		{"switch enter lf", []byte("\n"), keySwitch, 1},
		{"add a", []byte("a"), keyAdd, 1},
		{"remove d", []byte("d"), keyRemove, 1},
		{"refresh r", []byte("r"), keyRefresh, 1},
		{"quit q", []byte("q"), keyQuit, 1},
		{"quit ctrl-c", []byte{0x03}, keyQuit, 1},
		{"yes y", []byte("y"), keyYes, 1},
		{"no n", []byte("n"), keyNo, 1},
		{"unknown z", []byte("z"), keyNone, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, n := decodeKey(tc.in)
			if got != tc.want || n != tc.n {
				t.Fatalf("decodeKey(%q) = (%v, %d), want (%v, %d)", tc.in, got, n, tc.want, tc.n)
			}
		})
	}
}

func TestDecodeKeyArrows(t *testing.T) {
	if k, n := decodeKey([]byte{0x1b, '[', 'A'}); k != keyUp || n != 3 {
		t.Fatalf("up arrow = (%v,%d), want (keyUp,3)", k, n)
	}
	if k, n := decodeKey([]byte{0x1b, '[', 'B'}); k != keyDown || n != 3 {
		t.Fatalf("down arrow = (%v,%d), want (keyDown,3)", k, n)
	}
	// Left/right arrows are recognized as escape sequences but ignored.
	if k, n := decodeKey([]byte{0x1b, '[', 'C'}); k != keyNone || n != 3 {
		t.Fatalf("right arrow = (%v,%d), want (keyNone,3)", k, n)
	}
}

func TestDecodeKeyEmpty(t *testing.T) {
	if k, n := decodeKey(nil); k != keyNone || n != 0 {
		t.Fatalf("empty = (%v,%d), want (keyNone,0)", k, n)
	}
}

func TestDecodeKeyLoneEscape(t *testing.T) {
	if k, n := decodeKey([]byte{0x1b}); k != keyNone || n != 1 {
		t.Fatalf("lone esc = (%v,%d), want (keyNone,1)", k, n)
	}
}

func TestResolveActionNormalMode(t *testing.T) {
	cases := []struct {
		k    watchKey
		want watchAction
	}{
		{keyUp, actUp},
		{keyDown, actDown},
		{keySwitch, actSwitch},
		{keyAdd, actAdd},
		{keyRemove, actRemovePrompt},
		{keyRefresh, actRefresh},
		{keyQuit, actQuit},
		{keyYes, actNone},
		{keyNo, actNone},
		{keyNone, actNone},
	}
	for _, tc := range cases {
		if got := resolveAction(tc.k, false); got != tc.want {
			t.Fatalf("resolveAction(%v,false)=%v want %v", tc.k, got, tc.want)
		}
	}
}

func TestResolveActionConfirmMode(t *testing.T) {
	if got := resolveAction(keyYes, true); got != actConfirmRemove {
		t.Fatalf("yes in confirm = %v want actConfirmRemove", got)
	}
	if got := resolveAction(keyQuit, true); got != actQuit {
		t.Fatalf("quit in confirm = %v want actQuit", got)
	}
	// Anything else while confirming cancels the pending removal.
	for _, k := range []watchKey{keyNo, keyUp, keyDown, keySwitch, keyAdd, keyRemove, keyRefresh, keyNone} {
		if got := resolveAction(k, true); got != actCancelRemove {
			t.Fatalf("resolveAction(%v,true)=%v want actCancelRemove", k, got)
		}
	}
}

func TestClampIndex(t *testing.T) {
	cases := []struct{ i, n, want int }{
		{0, 0, -1},
		{5, 0, -1},
		{-3, 3, 0},
		{1, 3, 1},
		{5, 3, 2},
		{2, 3, 2},
	}
	for _, tc := range cases {
		if got := clampIndex(tc.i, tc.n); got != tc.want {
			t.Fatalf("clampIndex(%d,%d)=%d want %d", tc.i, tc.n, got, tc.want)
		}
	}
}

func TestReconcileSelectionKeepsPresent(t *testing.T) {
	ids := []string{"a", "b", "c"}
	if id, idx := reconcileSelection(ids, "b", 0); id != "b" || idx != 1 {
		t.Fatalf("got (%q,%d) want (b,1)", id, idx)
	}
}

func TestReconcileSelectionClampsWhenRemoved(t *testing.T) {
	// "b" was at index 1 and is now gone; the row at index 1 becomes "c".
	ids := []string{"a", "c"}
	if id, idx := reconcileSelection(ids, "b", 1); id != "c" || idx != 1 {
		t.Fatalf("got (%q,%d) want (c,1)", id, idx)
	}
}

func TestReconcileSelectionClampsPastEnd(t *testing.T) {
	ids := []string{"a"}
	if id, idx := reconcileSelection(ids, "gone", 3); id != "a" || idx != 0 {
		t.Fatalf("got (%q,%d) want (a,0)", id, idx)
	}
}

func TestReconcileSelectionEmpty(t *testing.T) {
	if id, idx := reconcileSelection(nil, "x", 2); id != "" || idx != -1 {
		t.Fatalf("got (%q,%d) want (\"\",-1)", id, idx)
	}
}

func TestMoveSelectionDown(t *testing.T) {
	ids := []string{"a", "b", "c"}
	if id, idx := moveSelection(ids, "a", 1); id != "b" || idx != 1 {
		t.Fatalf("got (%q,%d) want (b,1)", id, idx)
	}
}

func TestMoveSelectionClampsAtTop(t *testing.T) {
	ids := []string{"a", "b", "c"}
	if id, idx := moveSelection(ids, "a", -1); id != "a" || idx != 0 {
		t.Fatalf("got (%q,%d) want (a,0)", id, idx)
	}
}

func TestMoveSelectionClampsAtBottom(t *testing.T) {
	ids := []string{"a", "b"}
	if id, idx := moveSelection(ids, "b", 1); id != "b" || idx != 1 {
		t.Fatalf("got (%q,%d) want (b,1)", id, idx)
	}
}

func TestMoveSelectionUnknownStartsAtTop(t *testing.T) {
	ids := []string{"a", "b", "c"}
	if id, idx := moveSelection(ids, "missing", 1); id != "a" || idx != 0 {
		t.Fatalf("got (%q,%d) want (a,0)", id, idx)
	}
}

func TestMoveSelectionEmpty(t *testing.T) {
	if id, idx := moveSelection(nil, "", 1); id != "" || idx != -1 {
		t.Fatalf("got (%q,%d) want (\"\",-1)", id, idx)
	}
}
