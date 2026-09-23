package laya

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/safetensors"
)

type Shape struct {
	BatchSize  int  `json:"batch_size"`
	MaxLength  int  `json:"max_length"`
	MinLength  int  `json:"min_length"`
	MaxOptions int  `json:"max_options"`
	Flexible   bool `json:"flexible"`
}

type Manifest struct {
	Format        string `json:"format"`
	FormatVersion int    `json:"format_version"`
	Repository    string `json:"repository"`
	Shape         Shape  `json:"shape"`
}

type AgentConfig struct {
	MaxLen               int                `json:"max_len"`
	HeadMaxLen           int                `json:"head_max_len"`
	Temperature          []float64          `json:"temperature"`
	TemperatureByOptions map[string]float64 `json:"temperature_by_options"`
}

type HostWeights struct {
	Embedding     []float32 // [vocab, width]
	Vocab         int
	Width         int
	TypeEmb       []float32 // [3, width]
	ActionW0      []float32 // [256, 772]
	ActionB0      []float32 // [256]
	ActionW2      []float32 // [2, 256]
	ActionB2      []float32 // [2]
	mapped        *safetensors.Mapped
}

func loadManifest(dir string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, "coreml_config.json"))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	return m, nil
}

func loadAgentConfig(dir string) (AgentConfig, error) {
	var c AgentConfig
	b, err := os.ReadFile(filepath.Join(dir, "rl_agent_config.json"))
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.MaxLen == 0 {
		c.MaxLen = 512
	}
	if c.HeadMaxLen == 0 {
		c.HeadMaxLen = 192
	}
	if len(c.Temperature) != 3 {
		c.Temperature = []float64{1, 1, 1}
	}
	for i, t := range c.Temperature {
		c.Temperature[i] = clampTemp(t)
	}
	if c.TemperatureByOptions == nil {
		c.TemperatureByOptions = map[string]float64{}
	}
	for k, v := range c.TemperatureByOptions {
		c.TemperatureByOptions[k] = clampTemp(v)
	}
	return c, nil
}

func loadHostWeights(path string) (*HostWeights, error) {
	m := &safetensors.Mapped{}
	if err := m.Open(path); err != nil {
		return nil, err
	}
	byName := map[string]safetensors.Tensor{}
	for _, t := range m.Tensors {
		byName[t.Name] = t
	}
	need := []string{
		"encoder.embeddings.tok_embeddings.weight",
		"type_emb.weight",
		"act_head.0.weight",
		"act_head.0.bias",
		"act_head.2.weight",
		"act_head.2.bias",
	}
	for _, name := range need {
		if _, ok := byName[name]; !ok {
			_ = m.Close()
			return nil, fmt.Errorf("missing host weight %s", name)
		}
	}
	emb := byName["encoder.embeddings.tok_embeddings.weight"]
	if len(emb.Shape) != 2 {
		_ = m.Close()
		return nil, fmt.Errorf("bad embedding shape")
	}
	w := &HostWeights{
		mapped:    m,
		Vocab:     int(emb.Shape[0]),
		Width:     int(emb.Shape[1]),
		Embedding: f32FromF16Bytes(emb.Data),
		TypeEmb:   f32FromF16Bytes(byName["type_emb.weight"].Data),
		ActionW0:  f32FromF16Bytes(byName["act_head.0.weight"].Data),
		ActionB0:  f32FromF16Bytes(byName["act_head.0.bias"].Data),
		ActionW2:  f32FromF16Bytes(byName["act_head.2.weight"].Data),
		ActionB2:  f32FromF16Bytes(byName["act_head.2.bias"].Data),
	}
	return w, nil
}

func (w *HostWeights) Close() {
	if w != nil && w.mapped != nil {
		_ = w.mapped.Close()
		w.mapped = nil
	}
}
