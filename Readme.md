# Simple Linux Container in Go

A minimal container implementation demonstrating Linux namespaces, cgroups, chroot, and **overlay filesystems** (like Docker).

## Requirements

### Operating System
- **Linux only** - This program uses Linux-specific system calls (namespaces, cgroups, chroot)
- Will NOT work on Windows or macOS
- Tested on Ubuntu 22.04+

### Permissions
- **Must run as root** (`sudo`) - Container features require root privileges

### System Requirements
- Linux kernel 4.5+ (for cgroup v2 support)
- `debootstrap` package installed

---

## Setup Instructions

### Step 1: Install Dependencies

```bash
# On Ubuntu/Debian
sudo apt update
sudo apt install debootstrap

# On Fedora
sudo dnf install debootstrap

# On Arch Linux
sudo pacman -S debootstrap
```

### Step 2: Create the Container Root Filesystem

This creates a minimal Ubuntu filesystem that the container will use:

```bash
sudo debootstrap --variant=minbase jammy /root/rootfs
```

**What this does:**
- Downloads ~150MB of Ubuntu packages
- Creates a minimal Ubuntu 22.04 (jammy) system at `/root/rootfs`
- Takes 2-5 minutes depending on internet speed

**Alternative locations** (if you want to use a different path):
```bash
# Use a different directory
sudo debootstrap --variant=minbase jammy /path/to/your/rootfs

# Then update the const in main.go:
# const rootfs = "/path/to/your/rootfs"
```

### Step 3: Verify the Root Filesystem

```bash
# Check that basic files exist
ls /root/rootfs/bin/bash
ls /root/rootfs/bin/ls

# You should see these files exist
```

---

## Building the Container

```bash
# Navigate to the project directory
cd /home/contianer

# Build the Go program
go build -o container main.go
```

---

## Running the Container

### Basic Usage

```bash
sudo ./container run <command> [args...]
```

### Examples

```bash
# Start a bash shell inside the container
sudo ./container run /bin/bash

# Run a specific command
sudo ./container run /bin/ls -la /

# Check the hostname (should show "container")
sudo ./container run /bin/hostname

# See processes (only container processes visible)
sudo ./container run /bin/ps aux
```

### Inside the Container

Once inside, you'll notice:
- Hostname is `container`
- Only container processes are visible (`ps aux`)
- Filesystem is isolated to the rootfs
- Limited to 100MB RAM, 50% CPU, 20 processes
- **Changes are discarded on exit** (overlay filesystem)

To exit the container shell:
```bash
exit
```

### Overlay Filesystem Behavior (Like Docker)

The container uses an **overlay filesystem**:

```
┌─────────────────────────────────────────────┐
│  What the container sees (merged view)      │
├─────────────────────────────────────────────┤
│  Upper Layer (writable)                     │  ← Your changes go here
│  /root/container-overlay/upper/             │    (discarded on exit)
├─────────────────────────────────────────────┤
│  Lower Layer (read-only)                    │  ← Base Ubuntu system
│  /root/rootfs/                              │    (never modified)
└─────────────────────────────────────────────┘
```

**Example:**
```bash
# First run - create a file
sudo ./container run /bin/bash
root@container:/# echo "hello" > /myfile.txt
root@container:/# exit

# Second run - file is GONE (fresh start)
sudo ./container run /bin/bash
root@container:/# cat /myfile.txt
cat: /myfile.txt: No such file or directory
```

This keeps your base rootfs clean and ensures every container run starts fresh.

---

## Troubleshooting

### Error: "Rootfs not found"
```
Rootfs not found at /root/rootfs
Create it with: debootstrap --variant=minbase jammy /root/rootfs
```
**Solution:** Run the debootstrap command from Step 2.

### Error: "operation not permitted"
**Solution:** Make sure you're running with `sudo`:
```bash
sudo ./container run /bin/bash
```

### Error: "no such file or directory" for /bin/bash
**Solution:** The rootfs wasn't created properly. Re-run debootstrap:
```bash
sudo rm -rf /root/rootfs
sudo debootstrap --variant=minbase jammy /root/rootfs
```

### Error: cgroup-related errors
Your system might use cgroup v1 instead of v2. Check with:
```bash
mount | grep cgroup
```
If you see `cgroup2` on `/sys/fs/cgroup`, you have v2 (supported).

### Container has no internet
This is expected! The `CLONE_NEWNET` flag creates an isolated network namespace with no connectivity. To add networking, you would need to set up virtual ethernet pairs (veth) - not implemented in this basic example.

---

## What's Happening Under the Hood

| Feature | Linux Technology | Purpose |
|---------|------------------|---------|
| Process isolation | `CLONE_NEWPID` | Container sees only its own processes |
| Hostname isolation | `CLONE_NEWUTS` | Container has its own hostname |
| Filesystem isolation | `CLONE_NEWNS` + `chroot` | Container sees only its rootfs |
| Network isolation | `CLONE_NEWNET` | Container has separate network stack |
| Resource limits | cgroups v2 | Limits CPU, memory, processes |
| Copy-on-write | OverlayFS | Changes are discarded on exit (like Docker) |

---

## Project Structure

```
/home/contianer/
├── main.go              # Container implementation (requires root)
├── main_rootless.go     # Rootless container (no sudo needed)
├── container            # Compiled binary (after go build)
├── container-rootless   # Rootless binary (after go build)
└── README.md            # This file

/root/rootfs/            # Base filesystem for ROOT container
├── bin/
├── lib/
└── ...

~/container-rootfs/      # Base filesystem for ROOTLESS container
├── bin/
├── lib/
└── ...
```

---

## Cleanup

To remove everything:

```bash
# Remove the rootfs (frees ~300MB)
sudo rm -rf /root/rootfs

# Remove the overlay directories
sudo rm -rf /root/container-overlay

# Remove the cgroup (created when container runs)
sudo rmdir /sys/fs/cgroup/mycontainer

# Remove the binary
rm ./container
```

---

## Quick Start (Copy-Paste Commands)

```bash
# 1. Install debootstrap
sudo apt update && sudo apt install -y debootstrap

# 2. Create rootfs (takes a few minutes)
sudo debootstrap --variant=minbase jammy /root/rootfs

# 3. Build the container
cd /home/contianer
go build -o container main.go

# 4. Run it!
sudo ./container run /bin/bash
```

---

## Rootless Container (No sudo required!)

There's also a **rootless version** that runs without root privileges using user namespaces.

### How UID Mapping Works

```
┌─────────────────────────────────────────────────────────────┐
│  HOST                           CONTAINER                   │
│  ┌─────────────────┐           ┌─────────────────┐          │
│  │ UID 1000 (you)  │  ──────►  │ UID 0 (root)    │          │
│  │ GID 1000        │  mapping  │ GID 0 (root)    │          │
│  └─────────────────┘           └─────────────────┘          │
│                                                             │
│  You APPEAR to be root inside, but on the host you're      │
│  still your unprivileged user. Safe!                        │
└─────────────────────────────────────────────────────────────┘
```

### Build Rootless Version

```bash
go build -o container-rootless main_rootless.go
```

### Setup Rootfs for Rootless (Choose One Method)

**Option 1: Extract from Docker (Recommended)**
```bash
# Create rootfs directory
mkdir -p ~/container-rootfs

# Export an Alpine Linux image
docker export $(docker create alpine) | tar -C ~/container-rootfs -xf -
```

**Option 2: Extract from Docker (Ubuntu)**
```bash
mkdir -p ~/container-rootfs
docker export $(docker create ubuntu:22.04) | tar -C ~/container-rootfs -xf -
```

**Option 3: Download Alpine Mini Root FS**
```bash
mkdir -p ~/container-rootfs
cd ~/container-rootfs
wget https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.1-x86_64.tar.gz
tar -xzf alpine-minirootfs-3.19.1-x86_64.tar.gz
rm alpine-minirootfs-3.19.1-x86_64.tar.gz
```

### Run Rootless Container

```bash
# No sudo needed!
./container-rootless run /bin/sh

# Check you're "root" inside
id
# Output: uid=0(root) gid=0(root)

# But you can only access files your real user owns
```

### Rootless Limitations

| Feature | Root Container | Rootless Container |
|---------|---------------|-------------------|
| Network namespace | Full (can create interfaces) | Limited (no new interfaces) |
| OverlayFS | Works | Requires fuse-overlayfs |
| Cgroups | Full control | Limited (cgroupv2 + delegation) |
| Bind to ports < 1024 | Yes | No |
| Access all files | Yes | Only files you own |

### Enable User Namespaces (if needed)

Some systems disable unprivileged user namespaces. Enable with:

```bash
# Check if enabled
cat /proc/sys/kernel/unprivileged_userns_clone
# 1 = enabled, 0 = disabled

# Enable temporarily
sudo sysctl kernel.unprivileged_userns_clone=1

# Enable permanently
echo 'kernel.unprivileged_userns_clone=1' | sudo tee /etc/sysctl.d/userns.conf
sudo sysctl --system
```

### The Key Code Change

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Cloneflags: syscall.CLONE_NEWUSER | // ← This enables rootless!
                syscall.CLONE_NEWUTS |
                syscall.CLONE_NEWPID |
                syscall.CLONE_NEWNS,
    
    // Map your UID (e.g., 1000) to root (0) inside container
    UidMappings: []syscall.SysProcIDMap{
        {ContainerID: 0, HostID: os.Getuid(), Size: 1},
    },
    GidMappings: []syscall.SysProcIDMap{
        {ContainerID: 0, HostID: os.Getgid(), Size: 1},
    },
}
```

---

## References

- [Build Your Own Container Using Less than 100 Lines of Go](https://www.infoq.com/articles/build-a-container-golang/) - The original article this implementation is based on
