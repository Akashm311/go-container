package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Rootless container paths - uses $HOME instead of /root
var (
	homeDir       = os.Getenv("HOME")
	rootfs        = filepath.Join(homeDir, "container-rootfs")
	containerBase = filepath.Join(homeDir, ".container")
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: container run <command> [args...]")
		fmt.Println("\nThis is a ROOTLESS container - no sudo required!")
		fmt.Println("But you need to set up the rootfs first. See README.")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		parent()
	case "child":
		child()
	default:
		panic("Unknown command. Use: run")
	}
}

func parent() {
	fmt.Printf("Starting rootless container (your UID %d will be root inside)\n", os.Getuid())

	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)

	// Get current user's UID and GID
	uid := os.Getuid()
	gid := os.Getgid()

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | // New hostname
			syscall.CLONE_NEWPID | // New PID namespace
			syscall.CLONE_NEWNS | // New mount namespace
			syscall.CLONE_NEWUSER, // NEW: User namespace for rootless!
		// Note: CLONE_NEWNET removed - requires root to set up networking

		// Map current user (e.g., UID 1000) to root (UID 0) inside container
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,   // root inside container
				HostID:      uid, // your actual UID on host
				Size:        1,   // map just one user
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,   // root group inside container
				HostID:      gid, // your actual GID on host
				Size:        1,   // map just one group
			},
		},

		// Required for unprivileged user namespaces
		GidMappingsEnableSetgroups: false,
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func child() {
	// Now we're inside the new namespaces!
	// Our UID is mapped: we APPEAR to be root (UID 0) but we're not really

	// Set hostname
	must(syscall.Sethostname([]byte("container")))

	// Setup filesystem
	setupFilesystem()

	// Run the requested command
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func setupFilesystem() {
	// Check if rootfs exists
	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		fmt.Printf("ERROR: Rootfs not found at %s\n\n", rootfs)
		fmt.Println("For ROOTLESS containers, create rootfs with ONE of these methods:")
		fmt.Println("")
		fmt.Println("Option 1: Extract a Docker image (RECOMMENDED)")
		fmt.Println("  mkdir -p ~/container-rootfs")
		fmt.Println("  docker export $(docker create alpine) | tar -C ~/container-rootfs -xf -")
		fmt.Println("")
		fmt.Println("Option 2: Use debootstrap with fakeroot (slower)")
		fmt.Println("  fakeroot debootstrap --variant=minbase jammy ~/container-rootfs")
		fmt.Println("")
		fmt.Println("Running without chroot...")

		// Mount proc in current namespace
		must(syscall.Mount("proc", "/proc", "proc", 0, ""))
		return
	}

	// Make our mount namespace private (don't propagate to host)
	must(syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""))

	// Bind mount the rootfs to itself (required for pivot_root)
	must(syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND|syscall.MS_REC, ""))

	// Use pivot_root instead of chroot (more secure, works better with namespaces)
	// First, create a directory for the old root
	pivotDir := filepath.Join(rootfs, ".pivot_root")
	os.MkdirAll(pivotDir, 0755)

	// Pivot the root
	must(syscall.PivotRoot(rootfs, pivotDir))

	// Change to new root
	must(os.Chdir("/"))

	// Mount /proc
	os.MkdirAll("/proc", 0755)
	must(syscall.Mount("proc", "/proc", "proc", 0, ""))

	// Unmount the old root (it's now at /.pivot_root)
	must(syscall.Unmount("/.pivot_root", syscall.MNT_DETACH))

	// Remove the pivot directory
	os.RemoveAll("/.pivot_root")

	fmt.Println("Rootless container filesystem ready!")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
