#!/usr/bin/env bash
# =============================================================================
# apply-sysctl.sh — Áp dụng OS kernel tuning cho high-throughput benchmark
# Chạy với sudo: sudo bash scripts/benchmark/apply-sysctl.sh
# =============================================================================
set -euo pipefail

if [ "$EUID" -ne 0 ]; then
  echo "Script này cần quyền root. Chạy: sudo bash $0"
  exit 1
fi

echo "[sysctl] Applying network tuning for 40-50k RPS benchmark..."

# ---- TCP Connection Handling ----
sysctl -w net.core.somaxconn=65535
sysctl -w net.core.netdev_max_backlog=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535
sysctl -w net.ipv4.tcp_syncookies=1
sysctl -w net.ipv4.tcp_tw_reuse=1
sysctl -w net.ipv4.ip_local_port_range="1024 65535"
sysctl -w net.ipv4.tcp_fin_timeout=10
sysctl -w net.ipv4.tcp_keepalive_time=60
sysctl -w net.ipv4.tcp_keepalive_intvl=10
sysctl -w net.ipv4.tcp_keepalive_probes=6

# ---- Socket Buffer Sizes (128MB) ----
sysctl -w net.core.rmem_max=134217728
sysctl -w net.core.wmem_max=134217728
sysctl -w net.core.rmem_default=524288
sysctl -w net.core.wmem_default=524288
sysctl -w net.ipv4.tcp_rmem="4096 87380 134217728"
sysctl -w net.ipv4.tcp_wmem="4096 65536 134217728"

# ---- File Descriptors ----
sysctl -w fs.file-max=2097152

# ---- Virtual Memory ----
sysctl -w vm.swappiness=1            # Giảm swap tendency (không tắt hẳn để tránh OOM)
sysctl -w vm.dirty_ratio=40
sysctl -w vm.dirty_background_ratio=10

# ---- Tắt swap nếu có ----
if swapon --show | grep -q .; then
  echo "[sysctl] Tắt swap tạm thời (benchmark duration)..."
  swapoff -a
  echo "[sysctl] Swap đã tắt. Nhớ chạy 'sudo swapon -a' sau khi benchmark xong."
fi

echo ""
echo "[sysctl] ✅ All tuning applied successfully."
echo ""
echo "Sau khi benchmark xong, khôi phục swap bằng:"
echo "  sudo swapon -a"
