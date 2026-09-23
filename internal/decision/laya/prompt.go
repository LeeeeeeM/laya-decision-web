package laya

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

var qtypes = map[decision.QuestionType]int{
	decision.TypeChoice: 0,
	decision.TypeScore:  1,
	decision.TypeNoul:   2,
}

var qtypeNames = map[int]string{0: "choice", 1: "score", 2: "noul"}

type internalQuestion struct {
	Type         decision.QuestionType
	Instructions string
	CriteriaMap  map[string]string
	CriteriaList []string
}

type preparedItem struct {
	IDs     []int
	Markers []int
	QType   int
	Key     string
	Q       internalQuestion
}

func serializeState(raw json.RawMessage) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", err
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func renderCriterion(v string) string { return v }

func renderOptions(q internalQuestion) []string {
	switch q.Type {
	case decision.TypeChoice:
		out := make([]string, 0, len(q.CriteriaMap))
		for k, v := range q.CriteriaMap {
			if v == "" {
				out = append(out, k)
			} else {
				out = append(out, k+": "+renderCriterion(v))
			}
		}
		// Preserve map iteration instability — criteria must be ordered.
		// Caller should pass ordered keys via CriteriaMap built from ordered JSON.
		return out
	case decision.TypeScore:
		out := make([]string, len(q.CriteriaList))
		for i, c := range q.CriteriaList {
			out[i] = fmt.Sprintf("level %d: %s", i, renderCriterion(c))
		}
		return out
	default:
		return []string{
			"false: no, the statement does not hold",
			"true: yes, the statement holds",
		}
	}
}

func orderedChoiceLabels(raw json.RawMessage) ([]string, map[string]string, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, nil, fmt.Errorf("choice criteria must be object")
	}
	labels := []string{}
	m := map[string]string{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, _ := keyTok.(string)
		var val any
		if err := dec.Decode(&val); err != nil {
			return nil, nil, err
		}
		switch v := val.(type) {
		case string:
			m[key] = v
		case nil:
			m[key] = ""
		default:
			b, _ := json.Marshal(v)
			m[key] = string(b)
		}
		labels = append(labels, key)
	}
	return labels, m, nil
}

func toInternal(q decision.Question) (internalQuestion, []string, error) {
	iq := internalQuestion{Type: q.Type, Instructions: q.Instructions}
	switch q.Type {
	case decision.TypeChoice:
		labels, m, err := orderedChoiceLabels(q.Criteria)
		if err != nil || len(labels) == 0 {
			return iq, nil, fmt.Errorf("choice criteria must be a nonempty object")
		}
		iq.CriteriaMap = m
		return iq, labels, nil
	case decision.TypeScore:
		if len(q.CriteriaList) == 0 {
			var list []string
			if err := json.Unmarshal(q.Criteria, &list); err != nil || len(list) == 0 {
				return iq, nil, fmt.Errorf("score criteria must be a nonempty list")
			}
			iq.CriteriaList = list
		} else {
			iq.CriteriaList = q.CriteriaList
		}
		labels := make([]string, len(iq.CriteriaList))
		for i := range iq.CriteriaList {
			labels[i] = fmt.Sprintf("%d", i)
		}
		return iq, labels, nil
	case decision.TypeNoul:
		return iq, []string{"false", "true"}, nil
	default:
		return iq, nil, fmt.Errorf("unknown question type")
	}
}

func renderOptionsOrdered(q internalQuestion, labels []string) []string {
	switch q.Type {
	case decision.TypeChoice:
		out := make([]string, len(labels))
		for i, k := range labels {
			v := q.CriteriaMap[k]
			if v == "" {
				out[i] = k
			} else {
				out[i] = k + ": " + v
			}
		}
		return out
	case decision.TypeScore:
		return renderOptions(q)
	default:
		return renderOptions(q)
	}
}

func buildPrefix(tok *Tokenizer, q internalQuestion, labels []string, headMaxLen int) ([]int, []int) {
	opts := renderOptionsOrdered(q, labels)
	ins := strings.ReplaceAll(q.Instructions, tok.MASK, " ")
	headIDs := tok.Encode(fmt.Sprintf("%s question: %s", q.Type, ins), false)
	optIDs := make([][]int, len(opts))
	for i, opt := range opts {
		ids := tok.Encode(" "+strings.ReplaceAll(opt, tok.MASK, " "), false)
		if len(ids) > 48 {
			ids = ids[:48]
		}
		optIDs[i] = append([]int{tok.MASKID}, ids...)
	}
	optBudget := headMaxLen
	for _, o := range optIDs {
		optBudget -= len(o)
	}
	if optBudget < 16 {
		per := max(4, (headMaxLen-16)/max(1, len(optIDs)))
		for i := range optIDs {
			if len(optIDs[i]) > per {
				optIDs[i] = optIDs[i][:per]
			}
		}
		optBudget = headMaxLen
		for _, o := range optIDs {
			optBudget -= len(o)
		}
	}
	if len(headIDs) > max(8, optBudget) {
		headIDs = headIDs[:max(8, optBudget)]
	}
	ids := []int{tok.CLSID}
	ids = append(ids, headIDs...)
	ids = append(ids, tok.SEPID)
	markers := make([]int, 0, len(optIDs))
	for _, o := range optIDs {
		markers = append(markers, len(ids))
		ids = append(ids, o...)
	}
	ids = append(ids, tok.SEPID)
	return ids, markers
}

func buildSequence(tok *Tokenizer, state string, q internalQuestion, labels []string, maxLen, headMaxLen int) ([]int, []int) {
	ids, markers := buildPrefix(tok, q, labels, headMaxLen)
	room := max(0, maxLen-len(ids)-1)
	st := tok.Encode(strings.ReplaceAll(state, tok.MASK, " "), false)
	if len(st) > room {
		st = st[:room]
	}
	ids = append(ids, st...)
	ids = append(ids, tok.SEPID)
	if len(ids) > maxLen {
		ids = ids[:maxLen]
	}
	kept := markers[:0]
	for _, m := range markers {
		if m < maxLen {
			kept = append(kept, m)
		}
	}
	return ids, kept
}

func prepare(tok *Tokenizer, state string, questions map[string]decision.Question, maxLen, headMaxLen int) ([]preparedItem, error) {
	keys := make([]string, 0, len(questions))
	for key := range questions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Prefer snake demo order when present.
	preferred := []string{"move", "risk", "food"}
	ordered := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, k := range preferred {
		if _, ok := questions[k]; ok {
			ordered = append(ordered, k)
			seen[k] = true
		}
	}
	for _, k := range keys {
		if !seen[k] {
			ordered = append(ordered, k)
		}
	}

	items := make([]preparedItem, 0, len(ordered))
	for _, key := range ordered {
		q := questions[key]
		iq, labels, err := toInternal(q)
		if err != nil {
			return nil, fmt.Errorf("question %s: %w", key, err)
		}
		ids, markers := buildSequence(tok, state, iq, labels, maxLen, headMaxLen)
		if len(markers) != len(renderOptionsOrdered(iq, labels)) {
			return nil, fmt.Errorf("question %q has too many options for the token budget", key)
		}
		item := preparedItem{
			IDs: ids, Markers: markers, QType: qtypes[iq.Type], Key: key, Q: iq,
		}
		if iq.Type == decision.TypeChoice {
			item.Q.CriteriaList = labels
		}
		items = append(items, item)
	}
	return items, nil
}

type batchTensors struct {
	InputIDs      []int32
	AttentionMask []int32
	MarkerPos     []int32
	MarkerMask    []int32
	QType         []int32
	BatchSize     int
	Length        int
	MaxOptions    int
}

func collate(items []preparedItem, padID int, shape Shape) (batchTensors, error) {
	if len(items) == 0 || len(items) > shape.BatchSize {
		return batchTensors{}, fmt.Errorf("batch must contain 1..%d questions", shape.BatchSize)
	}
	length := 0
	for _, it := range items {
		if len(it.IDs) > length {
			length = len(it.IDs)
		}
		if len(it.Markers) > shape.MaxOptions {
			return batchTensors{}, fmt.Errorf("question exceeds max_options")
		}
	}
	if length > shape.MaxLength {
		return batchTensors{}, fmt.Errorf("input has %d tokens, export supports at most %d", length, shape.MaxLength)
	}
	if shape.Flexible {
		minLen := shape.MinLength
		length = min(shape.MaxLength, max(minLen, ((length+15)/16)*16))
	} else {
		length = shape.MaxLength
	}
	b, k := shape.BatchSize, shape.MaxOptions
	out := batchTensors{
		InputIDs:      make([]int32, b*length),
		AttentionMask: make([]int32, b*length),
		MarkerPos:     make([]int32, b*k),
		MarkerMask:    make([]int32, b*k),
		QType:         make([]int32, b),
		BatchSize:     b,
		Length:        length,
		MaxOptions:    k,
	}
	for i := range out.InputIDs {
		out.InputIDs[i] = int32(padID)
	}
	for row := 0; row < b; row++ {
		out.AttentionMask[row*length] = 1
	}
	for row, item := range items {
		n := len(item.IDs)
		for i := 0; i < n; i++ {
			out.InputIDs[row*length+i] = int32(item.IDs[i])
			out.AttentionMask[row*length+i] = 1
		}
		for i, m := range item.Markers {
			out.MarkerPos[row*k+i] = int32(m)
			out.MarkerMask[row*k+i] = 1
		}
		out.QType[row] = int32(item.QType)
	}
	return out, nil
}

func clampTemp(t float64) float64 {
	if math.IsNaN(t) || math.IsInf(t, 0) {
		return 1
	}
	if t < 0.5 {
		return 0.5
	}
	if t > 5 {
		return 5
	}
	return t
}

func tempBucket(qtype, k int) string {
	size := "11+"
	switch {
	case k <= 2:
		size = "2"
	case k <= 5:
		size = "3-5"
	case k <= 10:
		size = "6-10"
	}
	return qtypeNames[qtype] + ":" + size
}

func confidenceFromProbs(p []float64, k int) float64 {
	if k < 2 {
		return 1
	}
	ent := 0.0
	for i := 0; i < k; i++ {
		v := math.Max(p[i], 1e-12)
		ent -= v * math.Log(v)
	}
	c := 1 - ent/math.Log(float64(k))
	return math.Min(1, math.Max(0, c))
}

func f16BytesFromF32(vals []float32) []byte {
	out := make([]byte, len(vals)*2)
	for i, v := range vals {
		binary.LittleEndian.PutUint16(out[i*2:], f32ToF16(math.Float32bits(v)))
	}
	return out
}

func f32FromF16Bytes(b []byte) []float32 {
	n := len(b) / 2
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		h := binary.LittleEndian.Uint16(b[i*2:])
		out[i] = math.Float32frombits(f16ToF32(h))
	}
	return out
}

func f16ToF32(h uint16) uint32 {
	sign := uint32(h>>15) << 31
	exp := uint32((h >> 10) & 0x1f)
	mant := uint32(h & 0x3ff)
	switch exp {
	case 0:
		if mant == 0 {
			return sign
		}
		exp = 127 - 15 + 1
		for mant&0x400 == 0 {
			mant <<= 1
			exp--
		}
		mant &= 0x3ff
		return sign | (exp << 23) | (mant << 13)
	case 31:
		return sign | 0x7f800000 | (mant << 13)
	default:
		return sign | ((exp + (127 - 15)) << 23) | (mant << 13)
	}
}

func f32ToF16(f uint32) uint16 {
	sign := uint16((f >> 16) & 0x8000)
	exp := int((f>>23)&0xff) - 127 + 15
	mant := f & 0x7fffff
	switch {
	case exp <= 0:
		if exp < -10 {
			return sign
		}
		mant |= 0x800000
		shift := uint32(14 - exp)
		mant = (mant + (1 << (shift - 1))) >> shift
		return sign | uint16(mant)
	case exp >= 31:
		return sign | 0x7c00
	default:
		return sign | uint16(exp<<10) | uint16((mant+0x1000)>>13)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
