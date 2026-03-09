package main

import (
	"fmt"
	"strconv"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

var archNames = map[nvml.DeviceArchitecture]string{
	nvml.DEVICE_ARCH_KEPLER:  "Kepler",
	nvml.DEVICE_ARCH_MAXWELL: "Maxwell",
	nvml.DEVICE_ARCH_PASCAL:  "Pascal",
	nvml.DEVICE_ARCH_VOLTA:   "Volta",
	nvml.DEVICE_ARCH_TURING:  "Turing",
	nvml.DEVICE_ARCH_AMPERE:  "Ampere",
	nvml.DEVICE_ARCH_ADA:     "Ada",
	nvml.DEVICE_ARCH_HOPPER:  "Hopper",
}

var (
	gpuTemperature = prometheus.NewDesc(
		"nvidia_gpu_temperature_celsius",
		"GPU core temperature in celsius.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	memoryUsed = prometheus.NewDesc(
		"nvidia_gpu_memory_used_bytes",
		"GPU memory used in bytes.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	memoryFree = prometheus.NewDesc(
		"nvidia_gpu_memory_free_bytes",
		"GPU memory free in bytes.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	memoryTotal = prometheus.NewDesc(
		"nvidia_gpu_memory_total_bytes",
		"GPU memory total in bytes.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	powerDraw = prometheus.NewDesc(
		"nvidia_gpu_power_draw_watts",
		"GPU power draw in watts.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	powerLimit = prometheus.NewDesc(
		"nvidia_gpu_power_limit_watts",
		"GPU enforced power limit in watts.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	clockSM = prometheus.NewDesc(
		"nvidia_gpu_clock_sm_mhz",
		"GPU SM clock frequency in MHz.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	clockMemory = prometheus.NewDesc(
		"nvidia_gpu_clock_memory_mhz",
		"GPU memory clock frequency in MHz.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	gpuUtilization = prometheus.NewDesc(
		"nvidia_gpu_utilization_ratio",
		"GPU compute utilization as a ratio (0.0–1.0).",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	memoryUtilization = prometheus.NewDesc(
		"nvidia_gpu_memory_utilization_ratio",
		"GPU memory controller utilization as a ratio (0.0–1.0).",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	pcieTX = prometheus.NewDesc(
		"nvidia_gpu_pcie_tx_bytes_per_second",
		"GPU PCIe TX throughput in bytes per second.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	pcieRX = prometheus.NewDesc(
		"nvidia_gpu_pcie_rx_bytes_per_second",
		"GPU PCIe RX throughput in bytes per second.",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	fanSpeed = prometheus.NewDesc(
		"nvidia_gpu_fan_speed_ratio",
		"GPU fan speed as a ratio (0.0–1.0).",
		[]string{"gpu", "name", "uuid"}, nil,
	)
	gpuInfo = prometheus.NewDesc(
		"nvidia_gpu_info",
		"GPU device information, always 1.",
		[]string{"gpu", "name", "uuid", "driver_version", "cuda_version", "architecture"}, nil,
	)
)

var allDescs = []*prometheus.Desc{
	gpuTemperature, memoryUsed, memoryFree, memoryTotal,
	powerDraw, powerLimit, clockSM, clockMemory,
	gpuUtilization, memoryUtilization, pcieTX, pcieRX, fanSpeed, gpuInfo,
}

type NvidiaCollector struct {
	logger zerolog.Logger
}

func NewNvidiaCollector(logger zerolog.Logger) *NvidiaCollector {
	return &NvidiaCollector{logger: logger}
}

func (c *NvidiaCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range allDescs {
		ch <- d
	}
}

func (c *NvidiaCollector) Collect(ch chan<- prometheus.Metric) {
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		c.logger.Error().Str("error", nvml.ErrorString(ret)).Msg("failed to get device count")
		return
	}

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			c.logger.Error().Int("index", i).Str("error", nvml.ErrorString(ret)).Msg("failed to get device handle")
			continue
		}

		name, _ := device.GetName()
		uuid, _ := device.GetUUID()
		labels := []string{strconv.Itoa(i), name, uuid}

		c.collectInfo(ch, device, labels)
		c.collectTemperature(ch, device, labels)
		c.collectMemory(ch, device, labels)
		c.collectPower(ch, device, labels)
		c.collectClocks(ch, device, labels)
		c.collectUtilization(ch, device, labels)
		c.collectPCIe(ch, device, labels)
		c.collectFanSpeed(ch, device, labels)
	}
}

func (c *NvidiaCollector) collectInfo(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	driverVersion, _ := nvml.SystemGetDriverVersion()

	cudaVersion := "unknown"
	if v, ret := nvml.SystemGetCudaDriverVersion_v2(); ret == nvml.SUCCESS {
		cudaVersion = fmt.Sprintf("%d.%d", v/1000, (v%1000)/10)
	}

	architecture := "unknown"
	if arch, ret := device.GetArchitecture(); ret == nvml.SUCCESS {
		if name, ok := archNames[arch]; ok {
			architecture = name
		}
	}

	ch <- prometheus.MustNewConstMetric(gpuInfo, prometheus.GaugeValue, 1,
		append(labels, driverVersion, cudaVersion, architecture)...)
}

func (c *NvidiaCollector) collectTemperature(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get temperature")
		return
	}
	ch <- prometheus.MustNewConstMetric(gpuTemperature, prometheus.GaugeValue, float64(temp), labels...)
}

func (c *NvidiaCollector) collectMemory(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	mem, ret := device.GetMemoryInfo()
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get memory info")
		return
	}
	ch <- prometheus.MustNewConstMetric(memoryUsed, prometheus.GaugeValue, float64(mem.Used), labels...)
	ch <- prometheus.MustNewConstMetric(memoryFree, prometheus.GaugeValue, float64(mem.Free), labels...)
	ch <- prometheus.MustNewConstMetric(memoryTotal, prometheus.GaugeValue, float64(mem.Total), labels...)
}

func (c *NvidiaCollector) collectPower(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	power, ret := device.GetPowerUsage()
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get power usage")
	} else {
		ch <- prometheus.MustNewConstMetric(powerDraw, prometheus.GaugeValue, float64(power)/1000.0, labels...)
	}

	limit, ret := device.GetEnforcedPowerLimit()
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get power limit")
	} else {
		ch <- prometheus.MustNewConstMetric(powerLimit, prometheus.GaugeValue, float64(limit)/1000.0, labels...)
	}
}

func (c *NvidiaCollector) collectClocks(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	sm, ret := device.GetClockInfo(nvml.CLOCK_SM)
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get SM clock")
	} else {
		ch <- prometheus.MustNewConstMetric(clockSM, prometheus.GaugeValue, float64(sm), labels...)
	}

	mem, ret := device.GetClockInfo(nvml.CLOCK_MEM)
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get memory clock")
	} else {
		ch <- prometheus.MustNewConstMetric(clockMemory, prometheus.GaugeValue, float64(mem), labels...)
	}
}

func (c *NvidiaCollector) collectUtilization(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	util, ret := device.GetUtilizationRates()
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get utilization rates")
		return
	}
	ch <- prometheus.MustNewConstMetric(gpuUtilization, prometheus.GaugeValue, float64(util.Gpu)/100.0, labels...)
	ch <- prometheus.MustNewConstMetric(memoryUtilization, prometheus.GaugeValue, float64(util.Memory)/100.0, labels...)
}

func (c *NvidiaCollector) collectPCIe(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	tx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES)
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get PCIe TX throughput")
	} else {
		ch <- prometheus.MustNewConstMetric(pcieTX, prometheus.GaugeValue, float64(tx)*1024, labels...)
	}

	rx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES)
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get PCIe RX throughput")
	} else {
		ch <- prometheus.MustNewConstMetric(pcieRX, prometheus.GaugeValue, float64(rx)*1024, labels...)
	}
}

func (c *NvidiaCollector) collectFanSpeed(ch chan<- prometheus.Metric, device nvml.Device, labels []string) {
	speed, ret := device.GetFanSpeed()
	if ret != nvml.SUCCESS {
		c.logger.Debug().Str("gpu", labels[0]).Str("error", nvml.ErrorString(ret)).Msg("failed to get fan speed")
		return
	}
	ch <- prometheus.MustNewConstMetric(fanSpeed, prometheus.GaugeValue, float64(speed)/100.0, labels...)
}
