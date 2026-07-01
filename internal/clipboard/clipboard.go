// Package clipboard reads image data from the host platform's clipboard so the
// user can paste a screenshot (or any copied image) straight into a chat. The
// terminal itself can't hand us binary clipboard data, so — like internal/open
// — we shell out to the platform's clipboard utility and capture its stdout.
//
// Supported helpers:
//
//	linux (wayland): wl-paste --type image/png
//	linux (x11):     xclip -selection clipboard -t image/png -o
//	darwin:          osascript (writes the clipboard PNG to a temp file)
//	windows:         powershell Get-Clipboard -Format Image
//
// ReadImage returns the raw image bytes plus a detected MIME content type
// (e.g. "image/png"). ErrNoImage is returned when the clipboard holds no image
// so the UI can show a friendly notice rather than a raw tool error.
package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

// ErrNoImage indicates the clipboard does not currently contain an image.
var ErrNoImage = errors.New("clipboard: no image found")

// ErrNoClipboardTool indicates no supported clipboard helper was found on the
// host, so text could not be copied.
var ErrNoClipboardTool = errors.New("clipboard: no clipboard tool available")

// WriteText copies s to the host platform's clipboard so the user can paste it
// elsewhere. Like ReadImage it shells out to the platform's clipboard utility
// (there is no portable way for a terminal program to set the clipboard on
// every OS). ErrNoClipboardTool is returned when no helper is installed.
func WriteText(s string) error {
	return writePlatformText(s)
}

// writePlatformText dispatches to the right clipboard writer for the OS.
func writePlatformText(s string) error {
	switch runtime.GOOS {
	case "linux":
		return writeLinuxText(s)
	case "darwin":
		return runPipe(exec.Command("pbcopy"), s)
	case "windows":
		// clip.exe reads stdin and copies it to the Windows clipboard.
		return runPipe(exec.Command("clip"), s)
	default:
		return fmt.Errorf("clipboard: unsupported platform %q", runtime.GOOS)
	}
}

// writeLinuxText prefers Wayland's wl-copy, falling back to X11's xclip. If
// neither tool is installed, ErrNoClipboardTool is returned so the UI can show
// a friendly notice.
func writeLinuxText(s string) error {
	if path, err := exec.LookPath("wl-copy"); err == nil {
		return runPipe(exec.Command(path), s)
	}
	if path, err := exec.LookPath("xclip"); err == nil {
		return runPipe(exec.Command(path, "-selection", "clipboard"), s)
	}
	return ErrNoClipboardTool
}

// runPipe runs cmd, feeding s to its stdin. Used by the clipboard writers,
// which all read the text to copy from stdin.
func runPipe(cmd *exec.Cmd, s string) error {
	cmd.Stdin = bytes.NewBufferString(s)
	return cmd.Run()
}

// ReadImage returns the image currently on the clipboard and its MIME type.
// It tries the platform-appropriate helper(s); a missing helper or an empty
// clipboard yields ErrNoImage so callers can present a clear message.
func ReadImage() ([]byte, string, error) {
	data, err := readPlatformImage()
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", ErrNoImage
	}
	return data, detectContentType(data), nil
}

// readPlatformImage dispatches to the right clipboard reader for the OS.
func readPlatformImage() ([]byte, error) {
	switch runtime.GOOS {
	case "linux":
		return readLinuxImage()
	case "darwin":
		return readDarwinImage()
	case "windows":
		return readWindowsImage()
	default:
		return nil, fmt.Errorf("clipboard: unsupported platform %q", runtime.GOOS)
	}
}

// readLinuxImage prefers Wayland's wl-paste, falling back to X11's xclip. Both
// are asked for PNG specifically; if neither tool is installed or the clipboard
// holds no image, ErrNoImage is returned.
func readLinuxImage() ([]byte, error) {
	if path, err := exec.LookPath("wl-paste"); err == nil {
		out, err := runCapture(exec.Command(path, "--type", "image/png"))
		if err == nil && len(out) > 0 {
			return out, nil
		}
	}
	if path, err := exec.LookPath("xclip"); err == nil {
		out, err := runCapture(exec.Command(path, "-selection", "clipboard", "-t", "image/png", "-o"))
		if err == nil && len(out) > 0 {
			return out, nil
		}
	}
	return nil, ErrNoImage
}

// readDarwinImage uses AppleScript to write the clipboard's PNG representation
// to a temp file, then reads it back. osascript can't stream binary data on
// stdout, so the temp-file round-trip is the reliable path.
func readDarwinImage() ([]byte, error) {
	f, err := os.CreateTemp(os.TempDir(), "teams-tui-clip-*.png")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	script := fmt.Sprintf(
		`set theFile to (open for access POSIX file %q with write permission)
try
	write (the clipboard as «class PNGf») to theFile
	close access theFile
on error
	close access theFile
	error "no image"
end try`, path)

	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		return nil, ErrNoImage
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrNoImage
	}
	return data, nil
}

// readWindowsImage uses PowerShell to save the clipboard image to a temp file
// (PowerShell can't reliably stream image bytes on stdout) and reads it back.
func readWindowsImage() ([]byte, error) {
	f, err := os.CreateTemp(os.TempDir(), "teams-tui-clip-*.png")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	script := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms;`+
			`$img = [System.Windows.Forms.Clipboard]::GetImage();`+
			`if ($img -eq $null) { exit 1 };`+
			`$img.Save(%q, [System.Drawing.Imaging.ImageFormat]::Png)`, path)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err := cmd.Run(); err != nil {
		return nil, ErrNoImage
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrNoImage
	}
	return data, nil
}

// runCapture runs cmd and returns its stdout. stderr is discarded so a noisy
// helper (e.g. xclip printing "Error: target ... not available") doesn't leak
// into the captured image bytes.
func runCapture(cmd *exec.Cmd) ([]byte, error) {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// detectContentType returns the image MIME type for data, defaulting to
// image/png when http.DetectContentType can't recognise an image format (the
// clipboard helpers above all emit PNG).
func detectContentType(data []byte) string {
	ct := http.DetectContentType(data)
	switch ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return ct
	default:
		return "image/png"
	}
}
