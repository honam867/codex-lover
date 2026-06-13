package app

// watchKey is a decoded keypress from the interactive watch input stream.
type watchKey int

const (
	keyNone watchKey = iota
	keyUp
	keyDown
	keySwitch
	keyAdd
	keyRemove
	keyRefresh
	keyQuit
	keyYes
	keyNo
)

// watchAction is the resolved action for a key given the current UI mode.
type watchAction int

const (
	actNone watchAction = iota
	actUp
	actDown
	actSwitch
	actAdd
	actRemovePrompt
	actConfirmRemove
	actCancelRemove
	actRefresh
	actQuit
)

// decodeKey reads the first key from b and returns it plus the number of bytes
// consumed.
func decodeKey(b []byte) (watchKey, int) {
	if len(b) == 0 {
		return keyNone, 0
	}
	if b[0] == 0x1b { // ESC: arrow keys arrive as "\x1b[A" etc.
		if len(b) >= 3 && b[1] == '[' {
			switch b[2] {
			case 'A':
				return keyUp, 3
			case 'B':
				return keyDown, 3
			default:
				return keyNone, 3
			}
		}
		// Lone or incomplete escape sequence: consume one byte and ignore it.
		return keyNone, 1
	}
	switch b[0] {
	case 'k', 'K':
		return keyUp, 1
	case 'j', 'J':
		return keyDown, 1
	case 's', 'S', '\r', '\n':
		return keySwitch, 1
	case 'a', 'A':
		return keyAdd, 1
	case 'd', 'D':
		return keyRemove, 1
	case 'r', 'R':
		return keyRefresh, 1
	case 'q', 'Q', 0x03: // 'q' or Ctrl+C
		return keyQuit, 1
	case 'y', 'Y':
		return keyYes, 1
	case 'n', 'N':
		return keyNo, 1
	default:
		return keyNone, 1
	}
}

// resolveAction maps a key to an action, taking into account whether a remove
// confirmation is currently pending.
func resolveAction(k watchKey, confirming bool) watchAction {
	if confirming {
		switch k {
		case keyYes:
			return actConfirmRemove
		case keyQuit:
			return actQuit
		default:
			return actCancelRemove
		}
	}
	switch k {
	case keyUp:
		return actUp
	case keyDown:
		return actDown
	case keySwitch:
		return actSwitch
	case keyAdd:
		return actAdd
	case keyRemove:
		return actRemovePrompt
	case keyRefresh:
		return actRefresh
	case keyQuit:
		return actQuit
	default:
		return actNone
	}
}

// clampIndex clamps i into [0, n-1], or returns -1 when n == 0.
func clampIndex(i, n int) int {
	if n <= 0 {
		return -1
	}
	if i < 0 {
		return 0
	}
	if i > n-1 {
		return n - 1
	}
	return i
}

// indexOf returns the position of id in ids, or -1 when absent.
func indexOf(ids []string, id string) int {
	for i, candidate := range ids {
		if candidate == id {
			return i
		}
	}
	return -1
}

// reconcileSelection keeps the selected id when it still exists, otherwise it
// clamps the previous index into the new list. Returns ("", -1) when empty.
func reconcileSelection(ids []string, selectedID string, prevIndex int) (string, int) {
	if len(ids) == 0 {
		return "", -1
	}
	if i := indexOf(ids, selectedID); i >= 0 {
		return ids[i], i
	}
	idx := clampIndex(prevIndex, len(ids))
	return ids[idx], idx
}

// moveSelection moves the selection by delta and returns the new id and index.
func moveSelection(ids []string, selectedID string, delta int) (string, int) {
	if len(ids) == 0 {
		return "", -1
	}
	current := indexOf(ids, selectedID)
	if current < 0 {
		// Nothing valid selected yet: snap to the top instead of moving.
		return ids[0], 0
	}
	idx := clampIndex(current+delta, len(ids))
	return ids[idx], idx
}
