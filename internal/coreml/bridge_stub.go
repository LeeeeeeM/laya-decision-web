//go:build !darwin

package coreml

import "fmt"

type Model struct{}

const (
	UnitsAll    = 0
	UnitsCPU    = 1
	UnitsCPUGPU = 2
	UnitsCPUNE  = 3
)

type Input struct {
	Name  string
	Shape []int64
	Data  []byte
}

type Output struct {
	Name  string
	Shape []int64
	Data  []float32
}

func Load(path string, computeUnits int) (*Model, error) {
	return nil, fmt.Errorf("Core ML is only available on macOS (path=%s units=%d)", path, computeUnits)
}

func (m *Model) Close() {}

func (m *Model) Predict(inputs []Input) ([]Output, error) {
	return nil, fmt.Errorf("Core ML is only available on macOS")
}
