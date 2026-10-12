//go:build !linux || android

package clipboard

import "github.com/atotto/clipboard"

// readAll and writeAll delegate to atotto/clipboard, which uses the native
// clipboard on macOS (pbcopy/pbpaste) and Windows, and termux-clipboard-get/set
// on Android. Linux needs its own helper chain (see backend_linux.go): atotto
// locks onto a single X11 helper at package init, which is wrong on Wayland and
// in processes that start without a display environment.
func readAll() (string, error) {
	return clipboard.ReadAll()
}

func writeAll(text string) error {
	return clipboard.WriteAll(text)
}
