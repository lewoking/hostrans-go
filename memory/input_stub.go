//go:build !windows

package memory

import "fmt"

func FindGameWindow(pid uint32) (uintptr, string, error) {
	return 0, "", fmt.Errorf("not supported")
}

func FocusGame(pid uint32) error { return fmt.Errorf("not supported") }

func SendChat(pid uint32, text string) error { return fmt.Errorf("not supported") }

func CaptureChatInput(pid uint32) (string, error) { return "", fmt.Errorf("not supported") }

func PasteToGame(pid uint32, text string) error { return fmt.Errorf("not supported") }

func TranslateChatBox(pid uint32, translated string) error { return fmt.Errorf("not supported") }

func SetClipboardText(s string) error { return fmt.Errorf("not supported") }

func GetClipboardText() (string, error) { return "", fmt.Errorf("not supported") }
