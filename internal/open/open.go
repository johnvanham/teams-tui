// Package open shells out to the host platform's default opener so teams-tui
// can hand a file or URL off to the user's GUI environment. Terminals can't
// render images, so when the user wants to view a hosted image we download it
// (see SaveTempImage) and ask the OS to open it in the default image viewer or
// browser instead of trying to draw it in the TUI.
package open

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Open opens a file path or URL using the OS default handler:
//
//	linux:   xdg-open <target>
//	darwin:  open <target>
//	windows: cmd /c start "" <target>
//
// We use exec.Command(...).Start() rather than Run() so the call returns
// immediately: the GUI application (image viewer, browser) keeps running
// independently and the TUI is never blocked waiting on it. Any error from
// Start is returned. Unsupported platforms return an error.
func Open(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", target)
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		// The empty "" argument is start's window title; it keeps start from
		// treating a quoted target as the title.
		cmd = exec.Command("cmd", "/c", "start", "", target)
	default:
		return fmt.Errorf("open: unsupported platform %q", runtime.GOOS)
	}
	return cmd.Start()
}

// SaveTempImage writes data to a uniquely-named temp file in os.TempDir() and
// returns the full path. This lets the UI download a hosted image's bytes and
// then hand the resulting file to Open() for display. The extension is derived
// from name via filepath.Ext so the OS picks the right default viewer; when
// name carries no extension we default to ".png".
func SaveTempImage(data []byte, name string) (string, error) {
	ext := filepath.Ext(name)
	if ext == "" {
		ext = ".png"
	}
	// os.CreateTemp expands a single "*" in the pattern into random characters,
	// so "teams-tui-*.png" yields e.g. "teams-tui-1234567890.png".
	f, err := os.CreateTemp(os.TempDir(), "teams-tui-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	return f.Name(), nil
}
