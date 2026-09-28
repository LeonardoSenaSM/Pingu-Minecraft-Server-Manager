package manager

import (
	"errors"
	"fmt"
)

const (
	MemoryModeAutomatic = "automatic"
	MemoryModeManual    = "manual"
)

// MemoryPlan is the validated JVM heap configuration for one Paper process.
type MemoryPlan struct {
	Mode          string
	TotalGB       int
	RecommendedGB int
	InitialGB     int
	MaximumGB     int
}

// RecommendedMemoryGB returns a conservative heap size while preserving RAM
// for Windows, the GUI and native libraries.
func RecommendedMemoryGB(totalGB int) int {
	if totalGB <= 2 {
		return 1
	}
	reserveLimited := totalGB - 2
	var recommended int
	switch {
	case totalGB <= 4:
		recommended = 2
	case totalGB <= 8:
		recommended = 4
	case totalGB <= 16:
		recommended = 6
	default:
		recommended = 8
	}
	if recommended > reserveLimited {
		recommended = reserveLimited
	}
	if recommended < 1 {
		return 1
	}
	return recommended
}

func BuildMemoryPlan(totalGB int, mode string, manualGB int) (MemoryPlan, error) {
	if totalGB < 1 {
		return MemoryPlan{}, errors.New("memória física total inválida")
	}
	recommended := RecommendedMemoryGB(totalGB)
	maximum := recommended
	if mode == "" {
		mode = MemoryModeAutomatic
	}
	switch mode {
	case MemoryModeAutomatic:
	case MemoryModeManual:
		upper := totalGB - 2
		if upper < 1 {
			upper = 1
		}
		if manualGB < 1 || manualGB > upper {
			return MemoryPlan{}, fmt.Errorf("memória manual deve estar entre 1 e %d GB", upper)
		}
		maximum = manualGB
	default:
		return MemoryPlan{}, fmt.Errorf("modo de memória desconhecido: %s", mode)
	}
	initial := 1
	if maximum >= 4 {
		initial = 2
	}
	return MemoryPlan{
		Mode: mode, TotalGB: totalGB, RecommendedGB: recommended,
		InitialGB: initial, MaximumGB: maximum,
	}, nil
}

func (p MemoryPlan) JVMArgs() []string {
	return []string{fmt.Sprintf("-Xms%dG", p.InitialGB), fmt.Sprintf("-Xmx%dG", p.MaximumGB)}
}
