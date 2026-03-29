package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// rootfs is the path to the container's root filesystem (read-only base layer)
// Create this with: debootstrap --variant=minbase jammy /root/rootfs
const rootfs = "/root/rootfs"

// Overlay filesystem directories
const (
	overlayBase   = "/root/container-overlay"
	overlayUpper  = "/root/container-overlay/upper"  // Writable layer (changes go here)
	overlayWork   = "/root/container-overlay/work"   // Required by overlayfs
	overlayMerged = "/root/container-overlay/merged" // Combined view (what container sees)
)

// cgroup path for resource limits
const cgroupPath = "/sys/fs/cgroup/mycontainer"

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: main run <command> [args...]")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "run":
		parent()
	case "child":
		child()
	default:
		panic("wat should I do")
	}
}

func parent() {
	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}

	// Cleanup: remove cgroup after container exits
	cleanupCgroup()
}

func child() {
	// Setup cgroups for resource limits
	setupCgroups()

	// Setup hostname
	must(syscall.Sethostname([]byte("container")))

	// Setup filesystem isolation with chroot
	setupFilesystem()

	// Run the requested command
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
}

func setupFilesystem() {
	// Check if rootfs exists
	if _, err := os.Stat(rootfs); os.IsNotExist(err) {
		fmt.Printf("Rootfs not found at %s\n", rootfs)
		fmt.Println("Create it with: debootstrap --variant=minbase jammy /root/rootfs")
		fmt.Println("Running without chroot (host filesystem visible)...")
		
		// At minimum, mount proc in the current namespace
		must(syscall.Mount("proc", "/proc", "proc", 0, ""))
		return
	}

	// Setup overlay filesystem for copy-on-write behavior (like Docker)
	// This keeps the base rootfs clean - all changes go to the upper layer
	setupOverlayFS()

	// Change root to the merged overlay filesystem
	must(syscall.Chroot(overlayMerged))
	must(os.Chdir("/"))

	// Mount /proc inside the container (required for ps, top, etc.)
	must(syscall.Mount("proc", "/proc", "proc", 0, ""))
}

// setupOverlayFS creates an overlay filesystem with:
// - Lower layer: the base rootfs (read-only)
// - Upper layer: writable layer for changes (discarded on exit)
// - Merged: combined view that the container sees
func setupOverlayFS() {
	// Clean up any previous overlay directories
	os.RemoveAll(overlayBase)

	// Create overlay directories
	must(os.MkdirAll(overlayUpper, 0755))
	must(os.MkdirAll(overlayWork, 0755))
	must(os.MkdirAll(overlayMerged, 0755))

	// Mount the overlay filesystem
	// Format: "lowerdir=<ro-base>,upperdir=<rw-layer>,workdir=<work>"
	overlayOpts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", rootfs, overlayUpper, overlayWork)
	must(syscall.Mount("overlay", overlayMerged, "overlay", 0, overlayOpts))

	fmt.Println("Overlay filesystem mounted (changes will be discarded on exit)")
}

func setupCgroups() {
	// Create cgroup directory (cgroup v2)
	os.MkdirAll(cgroupPath, 0755)

	// Limit memory to 100MB
	os.WriteFile(filepath.Join(cgroupPath, "memory.max"), []byte("100000000"), 0644)

	// Limit CPU to 50% of one core (50000 out of 100000 microseconds)
	os.WriteFile(filepath.Join(cgroupPath, "cpu.max"), []byte("50000 100000"), 0644)

	// Limit to 20 processes
	os.WriteFile(filepath.Join(cgroupPath, "pids.max"), []byte("20"), 0644)

	// Add this process to the cgroup
	pid := os.Getpid()
	os.WriteFile(filepath.Join(cgroupPath, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0644)
}

func cleanupCgroup() {
	// Remove the cgroup directory
	// Note: cgroup must be empty (no processes) before removal
	if err := os.Remove(cgroupPath); err != nil {
		// Non-fatal: cgroup cleanup failure shouldn't crash
		fmt.Printf("Warning: could not remove cgroup: %v\n", err)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}