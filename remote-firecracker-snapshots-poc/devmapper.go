package main

import (
	"fmt"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func mountCtrSnap(ctrSnapDevPath string, readOnly bool) (string, error) {
	snapDevName := filepath.Base(ctrSnapDevPath)
	mountDir, err := os.MkdirTemp("", snapDevName)
	if err != nil {
		return "", err
	}
	mountDir = removeTrailingSlash(mountDir)

	err = mountExt4(ctrSnapDevPath, mountDir, readOnly)
	if err != nil {
		return "", errors.Wrapf(err, "mounting %s at %s", ctrSnapDevPath, mountDir)
	}
	return mountDir, nil
}

func mountExt4(devicePath, mountPath string, readOnly bool) error {
	// Do not update access times for (all types of) files on this filesystem.
	// Do not allow access to devices (special files) on this filesystem.
	// Do not allow programs to be executed from this filesystem.
	// Do not honor set-user-ID and set-group-ID bits or file  capabilities when executing programs from this filesystem.
	// Suppress the display of certain (printk()) warning messages in the kernel log.
	var flags uintptr = syscall.MS_NOATIME | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_SILENT
	options := make([]string, 0)

	if readOnly {
		// Mount filesystem read-only.
		flags |= syscall.MS_RDONLY
		options = append(options, "noload")
	}

	return syscall.Mount(devicePath, mountPath, "ext4", flags, strings.Join(options, ","))
}

func unMountExt4(mountPath string) error {
	return syscall.Unmount(mountPath, syscall.MNT_DETACH)
}

func unmountCtrSnap(ctrSnapMountPath string) error {
	err := unMountExt4(ctrSnapMountPath)
	if err != nil {
		return errors.Wrapf(err, "unmounting %s", ctrSnapMountPath)
	}

	err = os.RemoveAll(ctrSnapMountPath)
	if err != nil {
		return errors.Wrapf(err, "removing %s", ctrSnapMountPath)
	}
	return nil
}

func addTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	} else {
		return path + "/"
	}
}

func removeTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path[:len(path)-1]
	} else {
		return path
	}
}

func createPatch(imgMountPath, ctrSnapMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--only-write-batch=%s", patchPath)
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(imgMountPath), addTrailingSlash(ctrSnapMountPath))
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "creating patch between %s and %s at %s", imgMountPath, ctrSnapMountPath, patchPath)
	}

	err = os.Remove(patchPath + ".sh") // Remove unnecessary script output
	if err != nil {
		return errors.Wrapf(err, "removing %s", patchPath+".sh")
	}
	return nil
}

func applyPatch(ctrSnapMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--read-batch=%s", patchPath)
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(ctrSnapMountPath))
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "applying %s at %s", patchPath, ctrSnapMountPath)
	}
	return nil
}

func digHoles(filePath string) error {
	cmd := exec.Command("sudo", "fallocate", "--dig-holes", filePath)
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "digging holes in %s", filePath)
	}
	return nil
}
