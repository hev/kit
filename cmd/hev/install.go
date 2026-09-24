package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installBinCmd = &cobra.Command{
	Use:    "install-bin",
	Short:  "Install a built hev binary",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		src, _ := cmd.Flags().GetString("src")
		dest, _ := cmd.Flags().GetString("dest")
		src, dest, err := resolveInstallPaths(src, dest)
		if err != nil {
			return err
		}
		return installBuiltBinary(src, dest)
	},
}

func init() {
	installBinCmd.Flags().String("src", "", "built hev binary to install")
	installBinCmd.Flags().String("dest", "", "destination path")
	rootCmd.AddCommand(installBinCmd)
}

func resolveInstallPaths(src, dest string) (string, string, error) {
	if src == "" {
		return "", "", fmt.Errorf("--src is required")
	}
	if dest == "" {
		return "", "", fmt.Errorf("--dest is required")
	}
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return "", "", fmt.Errorf("resolve src: %w", err)
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return "", "", fmt.Errorf("resolve dest: %w", err)
	}
	return filepath.Clean(srcAbs), filepath.Clean(destAbs), nil
}

func installBuiltBinary(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		if !isPermissionError(err) {
			return fmt.Errorf("create install dir: %w", err)
		}
		return sudoInstallBuiltBinary(src, dest)
	}
	if err := copyFile(src, dest); err == nil {
		return nil
	} else if !isPermissionError(err) {
		return fmt.Errorf("install hev: %w", err)
	}

	return sudoInstallBuiltBinary(src, dest)
}

func sudoInstallBuiltBinary(src, dest string) error {
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("install hev: %w; sudo not found for privileged install", err)
	}
	fmt.Fprintf(os.Stderr, "Installing %s requires sudo to replace %s\n", src, dest)
	cmd := exec.Command(sudo, "install", "-d", "-m", "0755", filepath.Dir(dest))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo create install dir: %w", err)
	}
	cmd = exec.Command(sudo, "install", "-m", "0755", src, dest)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo install hev: %w", err)
	}
	return nil
}

func isPermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission)
}
