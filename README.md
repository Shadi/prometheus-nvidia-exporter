# prometheus-nvidia-exporter

Prometheus exporter for NVIDIA GPU metrics via NVML.

## Metrics


| Metric | Description |
|---|---|
| `nvidia_gpu_info` | Device info (driver_version, cuda_version, architecture). Always 1. |
| `nvidia_gpu_temperature_celsius` | GPU core temperature |
| `nvidia_gpu_memory_used_bytes` | VRAM used |
| `nvidia_gpu_memory_free_bytes` | VRAM free |
| `nvidia_gpu_memory_total_bytes` | VRAM total |
| `nvidia_gpu_utilization_ratio` | GPU compute utilization (0.0–1.0) |
| `nvidia_gpu_memory_utilization_ratio` | Memory controller utilization (0.0–1.0) |
| `nvidia_gpu_power_draw_watts` | Current power draw |
| `nvidia_gpu_power_limit_watts` | Enforced power limit |
| `nvidia_gpu_clock_sm_mhz` | SM clock frequency |
| `nvidia_gpu_clock_memory_mhz` | Memory clock frequency |
| `nvidia_gpu_pcie_tx_bytes_per_second` | PCIe TX throughput |
| `nvidia_gpu_pcie_rx_bytes_per_second` | PCIe RX throughput |
| `nvidia_gpu_fan_speed_ratio` | Fan speed (0.0–1.0) |

## NVML dependency

Uses [go-nvml](https://github.com/NVIDIA/go-nvml) — Go bindings for [NVML](https://developer.nvidia.com/management-library-nvml) — which calls `dlopen("libnvidia-ml.so.1")` at runtime. The exporter searches these paths in order:

1. `libnvidia-ml.so.1` (system default via `dlopen`)
2. `/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1`
3. `/usr/lib/aarch64-linux-gnu/libnvidia-ml.so.1`
4. `/usr/lib64/libnvidia-ml.so.1`
5. `/usr/lib/libnvidia-ml.so.1`
6. `/home/kubernetes/bin/nvidia/lib64/libnvidia-ml.so.1` (GKE COS nodes)

If the library is in a non-standard location, set `NVML_LIBRARY_PATH` with colon-separated paths to try before the defaults:

```yaml
env:
  - name: NVML_LIBRARY_PATH
    value: /opt/nvidia/lib64/libnvidia-ml.so.1
```

## Usage

```
nvidia-exporter [flags]

  -l, --listen-address  Address to listen on (default: :8082)
  -m, --metrics-path    Metrics path (default: /metrics)
      --log-level       Log level: debug, info, warn, error (default: info)
```

## Kubernetes (GKE)

Tested on GKE and an on-premise cluster, See `deploy/daemonset.yaml` for the daemonset used, Key requirements:

- **`privileged: true`** — NVML opens `/dev/nvidiactl` and `/dev/nvidia0` to communicate with the kernel driver. Without privileged mode, device access is denied.
- **`/dev` volume mount** — Exposes host GPU device nodes (`/dev/nvidia*`) to the container.
- **`/home/kubernetes/bin/nvidia/lib64` volume mount** — On GKE COS nodes, NVIDIA drivers are installed here instead of standard library paths.
- **`LD_LIBRARY_PATH`** — Set to the mounted driver path so `dlopen` can resolve `libnvidia-ml.so.1` and its dependencies.

The daemonset uses a node affinity with `cloud.google.com/gke-accelerator: Exists` to target any GKE node with a GPU, and tolerates the `nvidia.com/gpu=present:NoSchedule` taint. For non-GKE clusters, update these to match your GPU node labels and taints, e.g.:

```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: nvidia.com/gpu.present
              operator: Exists
tolerations:
  - key: nvidia.com/gpu
    operator: Exists
    effect: NoSchedule
```

## Docker / Podman

```
docker run --gpus all -p 8082:8082 ghcr.io/shadi/prometheus-nvidia-exporter:latest
```

For podman without CDI, mount the library and devices manually:

```
podman run -d \
  -v /usr/lib/libnvidia-ml.so.1:/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1:ro \
  --device /dev/nvidia0 --device /dev/nvidiactl --device /dev/nvidia-uvm \
  -p 8082:8082 nvidia-exporter
```
