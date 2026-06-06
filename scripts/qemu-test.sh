#!/bin/bash
# OpenWrt QEMU test script for FamilyWarden

set -e

IMG_DIR="images"
mkdir -p "$IMG_DIR"

# Image URLs
X86_IMG_URL="https://downloads.openwrt.org/releases/25.12.4/targets/x86/64/openwrt-25.12.4-x86-64-generic-ext4-combined.img.gz"
X86_IMG="$IMG_DIR/openwrt-x86-25.12.4.img"
ARM_KERNEL_URL="https://downloads.openwrt.org/snapshots/targets/armsr/armv8/openwrt-armsr-armv8-generic-initramfs-kernel.bin"
ARM_KERNEL="$IMG_DIR/openwrt-arm-initramfs"

ARCH="${1:-x86}"  # Default to x86

download_x86() {
    if [ ! -f "$X86_IMG" ]; then
        echo "Downloading x86-64 image..."
        wget -q --show-progress -O "${X86_IMG}.gz" "$X86_IMG_URL"
        gunzip "${X86_IMG}.gz"
        qemu-img resize -f raw "$X86_IMG" +512M
    fi
}

download_arm() {
    if [ ! -f "$ARM_KERNEL" ]; then
        echo "Downloading ARM initramfs..."
        wget -q --show-progress -O "$ARM_KERNEL" "$ARM_KERNEL_URL"
    fi
}

run_x86() {
    download_x86
    echo "Starting x86-64 QEMU VM..."
    echo "Web UI: http://localhost:8080/"
    echo "SSH: ssh -p 2222 root@localhost"
    echo ""
    echo "Note: Firewall may block SSH. Access console first:"
    echo "  Run QEMU without -nographic to get VGA console"
    echo "  Or fix firewall: uci set firewall.ssh.src=wan; uci commit; /etc/init.d/firewall restart"

    qemu-system-x86_64 \
        -nographic \
        -m 512 \
        -drive "file=${X86_IMG},format=raw,if=virtio" \
        -netdev user,id=net0,hostfwd=tcp::2222-:22,hostfwd=tcp::8080-:80 \
        -device virtio-net-pci,netdev=net0
}

run_arm() {
    download_arm
    echo "Starting ARM virt QEMU VM..."
    echo "Note: ARM initramfs may not include dropbear (SSH)"

    qemu-system-aarch64 \
        -nographic \
        -M virt \
        -m 512 \
        -kernel "$ARM_KERNEL" \
        -netdev user,id=net0,hostfwd=tcp::2222-:22,hostfwd=tcp::8080-:80 \
        -device virtio-net-pci,netdev=net0
}

case "$ARCH" in
    x86|x86-64) run_x86 ;;
    arm|aarch64) run_arm ;;
    *)
        echo "Usage: $0 [x86|arm]"
        exit 1
        ;;
esac
