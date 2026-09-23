package laya

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/coreml"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

type Provider struct {
	dir      string
	modelID  string
	manifest Manifest
	cfg      AgentConfig
	tok      *Tokenizer
	ane      *aneRuntime
	ready    bool
}

type Options struct {
	ModelDir     string
	ModelID      string
	ComputeUnits int // coreml.Units*
}

func Open(opts Options) (*Provider, error) {
	dir := opts.ModelDir
	if dir == "" {
		return nil, fmt.Errorf("model dir required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("model dir not found: %s", abs)
	}
	manifest, err := loadManifest(abs)
	if err != nil {
		return nil, err
	}
	if manifest.Format != "laya-coreml-ane" || manifest.FormatVersion != 1 {
		return nil, fmt.Errorf("unsupported model format %s v%d (ANE package required for this path)", manifest.Format, manifest.FormatVersion)
	}
	if runtime.GOOS != "darwin" {
		return &Provider{
			dir: abs, modelID: firstNonEmpty(opts.ModelID, manifest.Repository, filepath.Base(abs)),
			manifest: manifest, ready: false,
		}, nil
	}
	cfg, err := loadAgentConfig(abs)
	if err != nil {
		return nil, err
	}
	tok, err := LoadTokenizer(filepath.Join(abs, "tokenizer"))
	if err != nil {
		return nil, err
	}
	units := opts.ComputeUnits
	if units == 0 {
		units = coreml.UnitsCPUNE
	}
	ane, err := loadANE(abs, manifest.Shape, units)
	if err != nil {
		tok.Close()
		return nil, err
	}
	id := firstNonEmpty(opts.ModelID, manifest.Repository, filepath.Base(abs))
	return &Provider{
		dir: abs, modelID: id, manifest: manifest, cfg: cfg, tok: tok, ane: ane, ready: true,
	}, nil
}

func (p *Provider) Close() {
	if p == nil {
		return
	}
	if p.tok != nil {
		p.tok.Close()
	}
	if p.ane != nil {
		p.ane.Close()
	}
}

func (p *Provider) Name() string { return decision.ProviderLaya }

func (p *Provider) Available() bool { return p != nil && p.ready }

func (p *Provider) ModelInfo() decision.ModelInfo {
	return decision.ModelInfo{
		ID:           p.modelID,
		Ready:        p.ready,
		Runtime:      "coreml",
		ComputeUnits: "cpu_ne",
		Status:       map[bool]string{true: "ready", false: "unavailable"}[p.ready],
	}
}

func (p *Provider) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	_ = ctx
	if !p.Available() {
		return decision.Response{}, fmt.Errorf("laya provider unavailable")
	}
	start := time.Now()
	state, err := serializeState(req.State)
	if err != nil {
		return decision.Response{}, err
	}
	questions, err := decision.NormalizeQuestions(req.Questions)
	if err != nil {
		return decision.Response{}, err
	}
	// Preserve original criteria JSON for ordered choice labels.
	for k, q := range questions {
		if orig, ok := req.Questions[k]; ok {
			q.Criteria = orig.Criteria
			questions[k] = q
		}
	}
	maxLen := p.cfg.MaxLen
	if p.manifest.Shape.MaxLength > 0 && p.manifest.Shape.MaxLength < maxLen {
		maxLen = p.manifest.Shape.MaxLength
	}
	items, err := prepare(p.tok, state, questions, maxLen, p.cfg.HeadMaxLen)
	if err != nil {
		return decision.Response{}, err
	}
	answers := map[string]json.RawMessage{}
	inputTokens := 0
	for _, item := range items {
		inputTokens += len(item.IDs)
		batch, err := collate([]preparedItem{item}, p.tok.PADID, p.manifest.Shape)
		if err != nil {
			return decision.Response{}, err
		}
		logits, act, err := p.ane.forward(batch)
		if err != nil {
			return decision.Response{}, err
		}
		for _, v := range logits {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return decision.Response{}, fmt.Errorf("non-finite Core ML outputs")
			}
		}
		k := len(item.Markers)
		scale := p.cfg.Temperature[item.QType]
		if v, ok := p.cfg.TemperatureByOptions[tempBucket(item.QType, k)]; ok {
			scale = v
		}
		z := make([]float64, k)
		maxz := math.Inf(-1)
		for i := 0; i < k; i++ {
			z[i] = float64(logits[i]) / scale
			if z[i] > maxz {
				maxz = z[i]
			}
		}
		pvec := make([]float64, k)
		sum := 0.0
		for i := 0; i < k; i++ {
			pvec[i] = math.Exp(z[i] - maxz)
			sum += pvec[i]
		}
		for i := range pvec {
			pvec[i] /= sum
		}
		actP := softmax64([]float64{float64(act[0]), float64(act[1])})
		answer := map[string]any{
			"type":       string(item.Q.Type),
			"confidence": round4(confidenceFromProbs(pvec, k)),
			"action":     map[string]any{"act_probability": round4(actP[0])},
		}
		switch item.Q.Type {
		case decision.TypeChoice:
			labels := item.Q.CriteriaList
			probs := map[string]float64{}
			best := 0
			for i, lab := range labels {
				probs[lab] = round4(pvec[i])
				if pvec[i] > pvec[best] {
					best = i
				}
			}
			answer["choice"] = labels[best]
			answer["probabilities"] = probs
		case decision.TypeScore:
			score := 0.0
			probs := map[string]float64{}
			legend := map[string]string{}
			for i := 0; i < k; i++ {
				score += float64(i) * pvec[i]
				probs[fmt.Sprintf("%d", i)] = round4(pvec[i])
				legend[fmt.Sprintf("%d", i)] = item.Q.CriteriaList[i]
			}
			answer["score"] = round4(score)
			answer["probabilities"] = probs
			answer["legend"] = legend
		case decision.TypeNoul:
			answer["noul"] = round4(pvec[1])
			answer["confidence"] = round4(math.Max(pvec[1], 1-pvec[1]))
		}
		raw, _ := json.Marshal(answer)
		answers[item.Key] = raw
	}
	return decision.Response{
		Provider: decision.ProviderLaya,
		Model:    p.modelID,
		Answers:  answers,
		Usage:    decision.Usage{InputTokens: inputTokens},
		Timing:   decision.Timing{ProviderMS: float64(time.Since(start).Microseconds()) / 1000},
	}, nil
}

func softmax64(z []float64) []float64 {
	maxv := z[0]
	for _, v := range z[1:] {
		if v > maxv {
			maxv = v
		}
	}
	out := make([]float64, len(z))
	sum := 0.0
	for i, v := range z {
		out[i] = math.Exp(v - maxv)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
