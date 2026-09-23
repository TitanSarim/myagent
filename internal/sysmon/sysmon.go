package sysmon

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type Stats struct {
	CPUPercent float64
	RAMUsedGB  float64
	RAMTotalGB float64
	RAMPercent float64
	GPUPercent float64
	GPUMemUsed float64 // MiB
	GPUMemTot  float64 // MiB
	GPUName    string
	HasGPU     bool
	Err        string
}

func Sample() Stats {
	var s Stats

	if v, err := mem.VirtualMemory(); err == nil {
		s.RAMUsedGB = float64(v.Used) / (1024 * 1024 * 1024)
		s.RAMTotalGB = float64(v.Total) / (1024 * 1024 * 1024)
		s.RAMPercent = v.UsedPercent
	}
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		s.CPUPercent = pcts[0]
	}

	if g, ok := sampleGPU(); ok {
		s.HasGPU = true
		s.GPUPercent = g.util
		s.GPUMemUsed = g.memUsed
		s.GPUMemTot = g.memTotal
		s.GPUName = g.name
	}
	return s
}

type gpuSample struct {
	util     float64
	memUsed  float64
	memTotal float64
	name     string
}

func sampleGPU() (gpuSample, bool) {
	// nvidia-smi is the most reliable for this workstation.
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return gpuSample{}, false
	}
	line := strings.TrimSpace(out.String())
	if line == "" {
		return gpuSample{}, false
	}
	// Take first GPU line
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	parts := splitCSV(line)
	if len(parts) < 4 {
		return gpuSample{}, false
	}
	util, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	used, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	total, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	return gpuSample{
		name:     strings.TrimSpace(parts[0]),
		util:     util,
		memUsed:  used,
		memTotal: total,
	}, true
}

func splitCSV(s string) []string {
	var parts []string
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ',' && !inQ:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	parts = append(parts, cur.String())
	return parts
}

func Bar(pct float64, width int) string {
	if width < 4 {
		width = 4
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int((pct/100)*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (s Stats) Line() string {
	cpu := fmt.Sprintf("CPU %s %4.0f%%", Bar(s.CPUPercent, 8), s.CPUPercent)
	ram := fmt.Sprintf("RAM %s %0.1f/%0.0fG", Bar(s.RAMPercent, 8), s.RAMUsedGB, s.RAMTotalGB)
	if s.HasGPU {
		gPct := s.GPUPercent
		if s.GPUMemTot > 0 {
			// also show mem
		}
		gpu := fmt.Sprintf("GPU %s %4.0f%%  VRAM %0.0f/%0.0fM",
			Bar(gPct, 8), gPct, s.GPUMemUsed, s.GPUMemTot)
		return cpu + "   " + ram + "   " + gpu
	}
	return cpu + "   " + ram + "   GPU n/a"
}

// Tick interval for UI refresh.
const TickEvery = time.Second
