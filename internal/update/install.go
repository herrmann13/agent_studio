package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func install(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return installMac(path)
	case "linux":
		return installLinux(path)
	default:
		return fmt.Errorf("updates are not supported on %s", runtime.GOOS)
	}
}

func installLinux(path string) error {
	if filepath.Ext(path) != ".deb" {
		return fmt.Errorf("the Linux installer must be a .deb file")
	}
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return fmt.Errorf("pkexec is required to install the update")
	}
	apt, err := exec.LookPath("apt")
	if err != nil {
		return fmt.Errorf("apt is required to install the update")
	}
	output, err := exec.Command(pkexec, apt, "install", "-y", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not install update: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func installMac(dmgPath string) error {
	if filepath.Ext(dmgPath) != ".dmg" {
		return fmt.Errorf("the macOS installer must be a .dmg file")
	}
	appPath, err := currentMacAppPath()
	if err != nil {
		return err
	}
	script, err := os.CreateTemp("", "agent-studio-update-*.sh")
	if err != nil {
		return err
	}
	scriptPath := script.Name()
	if _, err := script.WriteString(`#!/bin/sh
while kill -0 "$1" 2>/dev/null; do sleep 1; done
tmp_dir=$(mktemp -d)
mount_dir="$tmp_dir/mount"
mkdir "$mount_dir"
/usr/bin/hdiutil attach -readonly -nobrowse -mountpoint "$mount_dir" "$2"
rm -rf "$3"
cp -R "$mount_dir/agent-studio.app" "$3"
/usr/bin/hdiutil detach "$mount_dir"
console_user=$(/usr/bin/stat -f %Su /dev/console)
if [ -n "$console_user" ] && [ "$console_user" != "root" ]; then
  /usr/bin/sudo -u "$console_user" /usr/bin/open "$3"
fi
rm -rf "$tmp_dir" "$2" "$0"
`); err != nil {
		script.Close()
		os.Remove(scriptPath)
		return err
	}
	if err := script.Close(); err != nil {
		os.Remove(scriptPath)
		return err
	}
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		os.Remove(scriptPath)
		return err
	}
	command := "/bin/sh " + shellQuote(scriptPath) + " " + strconv.Itoa(os.Getpid()) + " " + shellQuote(dmgPath) + " " + shellQuote(appPath) + " >/dev/null 2>&1 &"
	if err := exec.Command("/usr/bin/osascript", "-e", "do shell script "+strconv.Quote(command)+" with administrator privileges").Run(); err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("could not start the updater: %w", err)
	}
	return nil
}

func currentMacAppPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	marker := ".app/Contents/MacOS/"
	index := strings.Index(executable, marker)
	if index < 0 {
		return "", fmt.Errorf("move Agent Studio to Applications before using automatic updates")
	}
	return executable[:index+len(".app")], nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
