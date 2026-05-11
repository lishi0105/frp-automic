package monitor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"frps-status-app.local/status/src/diskmon"
)

// HostPressure 本机资源压力（用于父级告警；与「剩余空间 MB 阈值」的应急清理逻辑独立）。
type HostPressure struct {
	CPUOverload bool
	Load1       float64
	OnlineCPUs  int

	MemLow          bool
	MemAvailRatio   float64
	MemAvailBytes   uint64
	MemTotalBytes   uint64

	DiskLow        bool
	DiskFreeRatio  float64
	DiskFreeBytes  uint64
	DiskTotalBytes uint64

	ProbeErrors []string
}

const (
	cpuLoadRatioHigh   = 0.80 // load1 / OnlineCPUs > 0.8 视为过载
	memAvailRatioLow   = 0.10 // MemAvailable / MemTotal < 10%
	diskFreeRatioLow   = 0.10 // 分区剩余 / 总容量 < 10%
)

func procJoin(procRoot, name string) string {
	r := strings.TrimSpace(procRoot)
	if r == "" {
		r = "/proc"
	}
	return filepath.Join(r, strings.TrimPrefix(name, "/"))
}

// ProbeHostPressure 读取 /proc/loadavg、/proc/meminfo，以及 dbPath 所在分区空间。
// procRoot 为空时使用 /proc；容器内可配置为挂载宿主机的路径（如 /host/proc）。
func ProbeHostPressure(procRoot, dbPath string) (hp HostPressure) {
	hp.OnlineCPUs = runtime.NumCPU()
	if hp.OnlineCPUs < 1 {
		hp.OnlineCPUs = 1
	}

	loadPath := procJoin(procRoot, "loadavg")
	b, err := os.ReadFile(loadPath)
	if err != nil {
		hp.ProbeErrors = append(hp.ProbeErrors, fmt.Sprintf("读取 loadavg: %v", err))
	} else {
		load1, perr := parseLoadAvg1(string(b))
		if perr != nil {
			hp.ProbeErrors = append(hp.ProbeErrors, fmt.Sprintf("解析 loadavg: %v", perr))
		} else {
			hp.Load1 = load1
			if load1/float64(hp.OnlineCPUs) > cpuLoadRatioHigh {
				hp.CPUOverload = true
			}
		}
	}

	memPath := procJoin(procRoot, "meminfo")
	mb, err := os.ReadFile(memPath)
	if err != nil {
		hp.ProbeErrors = append(hp.ProbeErrors, fmt.Sprintf("读取 meminfo: %v", err))
	} else {
		totalKB, availKB, perr := parseMeminfoAvailTotal(string(mb))
		if perr != nil {
			hp.ProbeErrors = append(hp.ProbeErrors, fmt.Sprintf("解析 meminfo: %v", perr))
		} else if totalKB > 0 {
			hp.MemTotalBytes = totalKB * 1024
			hp.MemAvailBytes = availKB * 1024
			hp.MemAvailRatio = float64(availKB) / float64(totalKB)
			if hp.MemAvailRatio < memAvailRatioLow {
				hp.MemLow = true
			}
		}
	}

	free, total, err := diskmon.FreeAndTotalBytes(dbPath)
	if err != nil {
		hp.ProbeErrors = append(hp.ProbeErrors, fmt.Sprintf("分区空间: %v", err))
	} else {
		hp.DiskFreeBytes, hp.DiskTotalBytes = free, total
		if total > 0 {
			hp.DiskFreeRatio = float64(free) / float64(total)
		}
		hp.DiskLow = diskLowByRatio(free, total)
	}
	return hp
}

func diskLowByRatio(free, total uint64) bool {
	if total == 0 {
		return false
	}
	return float64(free)/float64(total) < diskFreeRatioLow
}

func parseLoadAvg1(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("空内容")
	}
	fields := strings.Fields(s)
	if len(fields) < 1 {
		return 0, fmt.Errorf("无字段")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// parseMeminfoAvailTotal 返回 MemTotal、MemAvailable（单位 kB）；无 MemAvailable 时用 MemFree 近似。
func parseMeminfoAvailTotal(content string) (totalKB, availKB uint64, err error) {
	var memTotal, memAvail, memFree *uint64
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		if after, ok := strings.CutPrefix(line, "MemTotal:"); ok {
			v, e := parseMeminfoKBytes(after)
			if e != nil {
				return 0, 0, e
			}
			memTotal = &v
		}
		if after, ok := strings.CutPrefix(line, "MemAvailable:"); ok {
			v, e := parseMeminfoKBytes(after)
			if e != nil {
				return 0, 0, e
			}
			memAvail = &v
		}
		if after, ok := strings.CutPrefix(line, "MemFree:"); ok {
			v, e := parseMeminfoKBytes(after)
			if e != nil {
				return 0, 0, e
			}
			memFree = &v
		}
	}
	if err := sc.Err(); err != nil {
		return 0, 0, err
	}
	if memTotal == nil {
		return 0, 0, fmt.Errorf("缺少 MemTotal")
	}
	avail := memAvail
	if avail == nil {
		avail = memFree
	}
	if avail == nil {
		return 0, 0, fmt.Errorf("缺少 MemAvailable/MemFree")
	}
	return *memTotal, *avail, nil
}

func parseMeminfoKBytes(rest string) (uint64, error) {
	fields := strings.Fields(strings.TrimSpace(rest))
	if len(fields) < 1 {
		return 0, fmt.Errorf("无数值")
	}
	return strconv.ParseUint(fields[0], 10, 64)
}
