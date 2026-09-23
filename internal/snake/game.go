package snake

import (
	"fmt"
	"math/rand"
)

var Directions = []string{"UP", "DOWN", "LEFT", "RIGHT"}

var vectors = map[string][2]int{
	"UP":    {0, -1},
	"DOWN":  {0, 1},
	"LEFT":  {-1, 0},
	"RIGHT": {1, 0},
}

type MoveInfo struct {
	Direction string `json:"direction"`
	Legal     bool   `json:"legal"`
	Safe      bool   `json:"safe"`
	Advance   int    `json:"advance"`
	Reason    string `json:"reason"`
	Eats      bool   `json:"eats"`
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Snapshot struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Seed        int64   `json:"seed"`
	Body        [][]int `json:"body"`
	Food        []int   `json:"food"`
	Score       int     `json:"score"`
	Length      int     `json:"length"`
	Ticks       int     `json:"ticks"`
	Alive       bool    `json:"alive"`
	Won         bool    `json:"won"`
	DeathReason string  `json:"death_reason,omitempty"`
}

type Game struct {
	Width         int
	Height        int
	Seed          int64
	Cycle         [][2]int
	Indices       map[[2]int]int
	Capacity      int
	InitialLength int
	rng           *rand.Rand
	Body          [][2]int
	Score         int
	Ticks         int
	Alive         bool
	Won           bool
	DeathReason   string
	Food          *[2]int
}

func HamiltonianCycle(width, height int) ([][2]int, error) {
	if min(width, height) < 4 || (width%2 == 1 && height%2 == 1) {
		return nil, fmt.Errorf("board dimensions must be >= 4, with at least one even dimension")
	}
	if height%2 == 1 {
		swapped, err := HamiltonianCycle(height, width)
		if err != nil {
			return nil, err
		}
		out := make([][2]int, len(swapped))
		for i, p := range swapped {
			out[i] = [2]int{p[1], p[0]}
		}
		return out, nil
	}
	path := [][2]int{{0, 0}}
	for y := 0; y < height; y++ {
		if y%2 == 0 {
			for x := 1; x < width; x++ {
				path = append(path, [2]int{x, y})
			}
		} else {
			for x := width - 1; x > 0; x-- {
				path = append(path, [2]int{x, y})
			}
		}
	}
	for y := height - 1; y > 0; y-- {
		path = append(path, [2]int{0, y})
	}
	return path, nil
}

func NewGame(width, height int, seed int64, initialLength int) (*Game, error) {
	cycle, err := HamiltonianCycle(width, height)
	if err != nil {
		return nil, err
	}
	capacity := width * height
	if initialLength < 2 || initialLength >= capacity {
		return nil, fmt.Errorf("initial length must be >= 2 and smaller than the board")
	}
	indices := make(map[[2]int]int, len(cycle))
	for i, cell := range cycle {
		indices[cell] = i
	}
	g := &Game{
		Width:         width,
		Height:        height,
		Seed:          seed,
		Cycle:         cycle,
		Indices:       indices,
		Capacity:      capacity,
		InitialLength: initialLength,
		rng:           rand.New(rand.NewSource(seed)),
		Alive:         true,
	}
	start := indices[[2]int{width / 2, height / 2}]
	g.Body = make([][2]int, initialLength)
	for i := 0; i < initialLength; i++ {
		g.Body[i] = cycle[(start-i+capacity)%capacity]
	}
	food := g.spawnFood()
	g.Food = food
	return g, nil
}

func (g *Game) Head() [2]int {
	return g.Body[0]
}

func (g *Game) spawnFood() *[2]int {
	occupied := make(map[[2]int]struct{}, len(g.Body))
	for _, c := range g.Body {
		occupied[c] = struct{}{}
	}
	empty := make([][2]int, 0, g.Capacity-len(g.Body))
	for _, cell := range g.Cycle {
		if _, ok := occupied[cell]; !ok {
			empty = append(empty, cell)
		}
	}
	if len(empty) == 0 {
		return nil
	}
	pick := empty[g.rng.Intn(len(empty))]
	return &pick
}

func (g *Game) Target(direction string) ([2]int, error) {
	v, ok := vectors[direction]
	if !ok {
		return [2]int{}, fmt.Errorf("unknown direction: %s", direction)
	}
	h := g.Head()
	return [2]int{h[0] + v[0], h[1] + v[1]}, nil
}

func (g *Game) LegalReason(direction string) string {
	cell, err := g.Target(direction)
	if err != nil {
		return "unknown"
	}
	x, y := cell[0], cell[1]
	if x < 0 || x >= g.Width || y < 0 || y >= g.Height {
		return "wall"
	}
	if len(g.Body) > 1 && cell == g.Body[1] {
		return "reverse"
	}
	occupied := make(map[[2]int]struct{}, len(g.Body))
	for _, c := range g.Body {
		occupied[c] = struct{}{}
	}
	if g.Food == nil || cell != *g.Food {
		delete(occupied, g.Body[len(g.Body)-1])
	}
	if _, ok := occupied[cell]; ok {
		return "body"
	}
	return "legal"
}

func (g *Game) Moves() []MoveInfo {
	if !g.Alive || g.Won || g.Food == nil {
		return nil
	}
	headIndex := g.Indices[g.Head()]
	tailDistance := (g.Indices[g.Body[len(g.Body)-1]] - headIndex + g.Capacity) % g.Capacity
	foodDistance := (g.Indices[*g.Food] - headIndex + g.Capacity) % g.Capacity
	moves := make([]MoveInfo, 0, 4)
	for _, direction := range Directions {
		reason := g.LegalReason(direction)
		legal := reason == "legal"
		target, _ := g.Target(direction)
		advance := 0
		if idx, ok := g.Indices[target]; ok {
			advance = (idx - headIndex + g.Capacity) % g.Capacity
		}
		eats := g.Food != nil && target == *g.Food
		safe := legal
		if safe && (advance > tailDistance || (advance == tailDistance && eats)) {
			safe = false
			reason = "would cross the tail"
		}
		if safe && (advance == 0 || advance > foodDistance) {
			safe = false
			reason = "would skip the food on the safe route"
		}
		moves = append(moves, MoveInfo{
			Direction: direction,
			Legal:     legal,
			Safe:      safe,
			Advance:   advance,
			Reason:    reason,
			Eats:      eats,
		})
	}
	return moves
}

func (g *Game) FoodReachability() (reachable bool, space int) {
	blocked := make(map[[2]int]struct{}, len(g.Body))
	for i, c := range g.Body {
		if i == 0 {
			continue
		}
		blocked[c] = struct{}{}
	}
	visited := map[[2]int]struct{}{g.Head(): {}}
	queue := [][2]int{g.Head()}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, v := range vectors {
			cell := [2]int{cur[0] + v[0], cur[1] + v[1]}
			if cell[0] < 0 || cell[0] >= g.Width || cell[1] < 0 || cell[1] >= g.Height {
				continue
			}
			if _, ok := blocked[cell]; ok {
				continue
			}
			if _, ok := visited[cell]; ok {
				continue
			}
			visited[cell] = struct{}{}
			queue = append(queue, cell)
		}
	}
	if g.Food != nil {
		_, reachable = visited[*g.Food]
	}
	return reachable, len(visited)
}

func (g *Game) Step(direction string) (bool, error) {
	if !g.Alive || g.Won {
		return false, fmt.Errorf("cannot step a finished game")
	}
	if _, ok := vectors[direction]; !ok {
		return false, fmt.Errorf("unknown direction: %s", direction)
	}
	g.Ticks++
	reason := g.LegalReason(direction)
	if reason != "legal" {
		g.Alive = false
		g.DeathReason = reason
		return false, nil
	}
	target, _ := g.Target(direction)
	g.Body = append([][2]int{target}, g.Body...)
	if g.Food != nil && target == *g.Food {
		g.Score++
		if len(g.Body) == g.Capacity {
			g.Won = true
			g.Food = nil
		} else {
			g.Food = g.spawnFood()
		}
		return true, nil
	}
	g.Body = g.Body[:len(g.Body)-1]
	return false, nil
}

func (g *Game) Snapshot() Snapshot {
	body := make([][]int, len(g.Body))
	for i, c := range g.Body {
		body[i] = []int{c[0], c[1]}
	}
	var food []int
	if g.Food != nil {
		food = []int{g.Food[0], g.Food[1]}
	}
	return Snapshot{
		Width:       g.Width,
		Height:      g.Height,
		Seed:        g.Seed,
		Body:        body,
		Food:        food,
		Score:       g.Score,
		Length:      len(g.Body),
		Ticks:       g.Ticks,
		Alive:       g.Alive,
		Won:         g.Won,
		DeathReason: g.DeathReason,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
