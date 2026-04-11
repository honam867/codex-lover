//go:build !windows

package notify

func send(event Event) error {
	return nil
}
