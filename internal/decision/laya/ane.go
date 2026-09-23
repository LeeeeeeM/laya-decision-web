package laya

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/LeeeeeeM/laya-decision-web/internal/coreml"
)

type aneRuntime struct {
	model   *coreml.Model
	weights *HostWeights
	width   int
	length  int
	window  []bool // length*length, row-major |i-j| <= local/2
}

func loadANE(dir string, shape Shape, computeUnits int) (*aneRuntime, error) {
	cfgPath := filepath.Join(dir, "encoder", "config.json")
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var enc struct {
		HiddenSize     int `json:"hidden_size"`
		LocalAttention int `json:"local_attention"`
	}
	if err := json.Unmarshal(b, &enc); err != nil {
		return nil, err
	}
	if enc.LocalAttention == 0 {
		enc.LocalAttention = 128
	}
	weights, err := loadHostWeights(filepath.Join(dir, "host_weights.safetensors"))
	if err != nil {
		return nil, err
	}
	units := coreml.UnitsCPUNE
	switch computeUnits {
	case coreml.UnitsCPU, coreml.UnitsCPUGPU, coreml.UnitsAll, coreml.UnitsCPUNE:
		units = computeUnits
	}
	pkg := filepath.Join(dir, "model.mlpackage")
	model, err := coreml.Load(pkg, units)
	if err != nil {
		weights.Close()
		return nil, err
	}
	length := shape.MaxLength
	half := enc.LocalAttention / 2
	window := make([]bool, length*length)
	for i := 0; i < length; i++ {
		for j := 0; j < length; j++ {
			d := i - j
			if d < 0 {
				d = -d
			}
			window[i*length+j] = d <= half
		}
	}
	return &aneRuntime{
		model:   model,
		weights: weights,
		width:   enc.HiddenSize,
		length:  length,
		window:  window,
	}, nil
}

func (a *aneRuntime) Close() {
	if a == nil {
		return
	}
	if a.model != nil {
		a.model.Close()
	}
	if a.weights != nil {
		a.weights.Close()
	}
}

func (a *aneRuntime) forward(batch batchTensors) (logits []float32, action []float32, err error) {
	if batch.BatchSize != 1 || batch.Length != a.length {
		return nil, nil, fmt.Errorf("ANE requires batch=1 length=%d", a.length)
	}
	ids := batch.InputIDs
	valid := batch.AttentionMask
	L := a.length
	W := a.width

	// embeddings layout [1, W, 1, L]
	emb := make([]float32, W*L)
	for t := 0; t < L; t++ {
		id := int(ids[t])
		if id < 0 || id >= a.weights.Vocab {
			return nil, nil, fmt.Errorf("token id outside vocabulary")
		}
		base := id * W
		for w := 0; w < W; w++ {
			emb[w*L+t] = a.weights.Embedding[base+w]
		}
	}

	// full mask bool [L,L]: full[q,k] = valid[k]; then transpose to [key,query] => full_t[k,q]=valid[k]
	// Python: full = broadcast valid[:, None, :] -> [B, Lq, Lk] with full[b,q,k]=valid[b,k]
	// then transpose(0,2,1) -> [B, Lk, Lq], then [:,:,None,:] -> [B,key,1,query]
	fullMask := make([]float32, L*L) // layout [key, query] flattened as key*L+query
	localMask := make([]float32, L*L)
	for k := 0; k < L; k++ {
		vk := valid[k] != 0
		for q := 0; q < L; q++ {
			full := vk
			local := (a.window[q*L+k] || valid[q] == 0) && full
			idx := k*L + q
			if full {
				fullMask[idx] = 0
			} else {
				fullMask[idx] = -1e4
			}
			if local {
				localMask[idx] = 0
			} else {
				localMask[idx] = -1e4
			}
		}
	}

	qtype := int(batch.QType[0])
	typeVec := make([]float32, W)
	copy(typeVec, a.weights.TypeEmb[qtype*W:(qtype+1)*W])

	markerMap := make([]float32, L*32) // [L, 1, 32] -> [L*32]
	for i := 0; i < 32; i++ {
		pos := int(batch.MarkerPos[i])
		if pos >= 0 && pos < L {
			markerMap[pos*32+i] = 1
		}
	}

	inputs := []coreml.Input{
		{Name: "embeddings", Shape: []int64{1, int64(W), 1, int64(L)}, Data: f16BytesFromF32(emb)},
		{Name: "full_mask", Shape: []int64{1, int64(L), 1, int64(L)}, Data: f16BytesFromF32(fullMask)},
		{Name: "local_mask", Shape: []int64{1, int64(L), 1, int64(L)}, Data: f16BytesFromF32(localMask)},
		{Name: "type_vectors", Shape: []int64{1, int64(W), 1, 1}, Data: f16BytesFromF32(typeVec)},
		{Name: "marker_map", Shape: []int64{1, int64(L), 1, 32}, Data: f16BytesFromF32(markerMap)},
	}
	outs, err := a.model.Predict(inputs)
	if err != nil {
		return nil, nil, err
	}
	var logitOut, pooledOut []float32
	for _, o := range outs {
		if len(o.Shape) >= 2 && o.Shape[1] == 1 {
			logitOut = o.Data
		} else {
			pooledOut = o.Data
		}
	}
	if logitOut == nil || pooledOut == nil {
		return nil, nil, fmt.Errorf("unexpected Core ML outputs")
	}
	logits = make([]float32, 32)
	copy(logits, logitOut)
	for i := 0; i < 32; i++ {
		if batch.MarkerMask[i] == 0 {
			logits[i] = -1e4
		}
	}

	// action head
	p := softmax32(logits)
	kCount := float32(0)
	for i := 0; i < 32; i++ {
		kCount += float32(batch.MarkerMask[i])
	}
	if kCount < 2 {
		kCount = 2
	}
	entropy := float32(0)
	for i := 0; i < 32; i++ {
		if batch.MarkerMask[i] == 0 {
			continue
		}
		pi := float64(p[i])
		if pi < 1e-9 {
			pi = 1e-9
		}
		entropy -= float32(pi * math.Log(pi))
	}
	entropy /= float32(math.Log(float64(kCount)))
	top1, top2 := float32(0), float32(0)
	for i := 0; i < 32; i++ {
		if batch.MarkerMask[i] == 0 {
			continue
		}
		if p[i] >= top1 {
			top2 = top1
			top1 = p[i]
		} else if p[i] > top2 {
			top2 = p[i]
		}
	}
	features := []float32{top1, top1 - top2, entropy, kCount / 255}
	pooled := pooledOut
	actionIn := append(append([]float32{}, pooled...), features...)
	hidden := matvec(a.weights.ActionW0, a.weights.ActionB0, actionIn, 256, 772)
	for i := range hidden {
		x := float64(hidden[i])
		hidden[i] = float32(x * (1 + math.Erf(x/math.Sqrt2)) / 2)
	}
	action = matvec(a.weights.ActionW2, a.weights.ActionB2, hidden, 2, 256)
	return logits, action, nil
}

func softmax32(z []float32) []float32 {
	maxv := z[0]
	for _, v := range z[1:] {
		if v > maxv {
			maxv = v
		}
	}
	out := make([]float32, len(z))
	sum := float32(0)
	for i, v := range z {
		out[i] = float32(math.Exp(float64(v - maxv)))
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func matvec(w, b, x []float32, rows, cols int) []float32 {
	out := make([]float32, rows)
	for r := 0; r < rows; r++ {
		sum := b[r]
		off := r * cols
		for c := 0; c < cols; c++ {
			sum += w[off+c] * x[c]
		}
		out[r] = sum
	}
	return out
}
