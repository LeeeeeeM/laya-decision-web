package platform

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

type Enemy struct {
	X, Y  float64
	Dir   float64 // +1 right, -1 left (patrol; not chase)
	Alive bool
}

type Bullet struct {
	X, Y float64
}

type Box struct {
	X, Y float64
	Hit  bool
}

type Item struct {
	X, Y  float64
	Alive bool // false after crouch-pickup
}

type Intent struct {
	Move   MoveIntent
	Action ActionIntent
}

type Snapshot struct {
	Seed       int64   `json:"seed"`
	CameraX    float64 `json:"camera_x"`
	ViewW      float64 `json:"view_w"`
	ViewH      float64 `json:"view_h"`
	TilePx     float64 `json:"tile_px"`
	GroundY    float64 `json:"ground_y"`
	PlayerX    float64 `json:"player_x"`
	PlayerY    float64 `json:"player_y"`
	PlayerVX   float64 `json:"player_vx"`
	PlayerVY   float64 `json:"player_vy"`
	PlayerW    float64 `json:"player_w"`
	PlayerH    float64 `json:"player_h"`
	Facing     int     `json:"facing"` // +1 right, -1 left
	Crouch     bool    `json:"crouch"`
	Grounded   bool    `json:"grounded"`
	Alive      bool    `json:"alive"`
	Score      int     `json:"score"`
	Distance   int     `json:"distance"`
	Ticks      int64   `json:"ticks"`
	DeathReason string `json:"death_reason,omitempty"`
	Move       string  `json:"move"`
	Action     string  `json:"action"`
	Solids     []int   `json:"solids"` // visible solid columns (world tile x)
	Pits       [][2]float64 `json:"pits"` // [start,end) visible
	Enemies    []EnemySnap `json:"enemies"`
	Bullets    []BulletSnap `json:"bullets"`
	Boxes      []BoxSnap  `json:"boxes"`
	Items      []ItemSnap `json:"items"`
}

type EnemySnap struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Facing float64 `json:"facing"` // +1 right, -1 left
	Alive  bool    `json:"alive"`
}
type BulletSnap struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type BoxSnap struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Hit bool    `json:"hit"`
}
type ItemSnap struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Alive bool    `json:"alive"`
}

type Game struct {
	Seed   int64
	rng    *rand.Rand
	intent Intent

	CameraX float64
	PlayerX float64
	PlayerY float64
	VX, VY  float64
	Facing  int // +1 right, -1 left — last intentional look direction
	Grounded bool
	Alive    bool
	Score    int
	Ticks    int64
	DeathReason string
	lastChunkScore int

	// solid[tileX] = true if ground present
	solid map[int]bool
	genRight int
	enemies  []Enemy
	bullets  []Bullet
	boxes    []Box
	items    []Item
	emptyStreak int
}

func NewGame(seed int64) *Game {
	if seed == 0 {
		seed = 7
	}
	g := &Game{
		Seed:     seed,
		rng:      rand.New(rand.NewSource(seed)),
		intent:   Intent{Move: MoveRight, Action: ActionNone},
		CameraX:  0,
		PlayerX:  4,
		PlayerY:  GroundY,
		Facing:   1,
		Grounded: true,
		Alive:    true,
		solid:    map[int]bool{},
		genRight: 0,
	}
	// Short safe runway, then procedural chunks (seeded RNG — no fixed showcase).
	for g.genRight < ChunkTiles {
		g.generateChunk()
	}
	need := int(ViewW + Lookahead + ChunkTiles)
	for g.genRight < need {
		g.generateChunk()
	}
	return g
}

func (g *Game) SetIntent(move MoveIntent, action ActionIntent) {
	if move != "" {
		g.intent.Move = move
	}
	if action != "" {
		// Jump/crouch only apply on the ground. Queued JUMP in air would
		// sticky-bounce on landing; match one-shot jump semantics for human+agent.
		if !g.Grounded && (action == ActionJump || action == ActionCrouch) {
			action = ActionNone
		}
		g.intent.Action = action
	}
}

func (g *Game) Intent() Intent { return g.intent }

func (g *Game) playerH() float64 {
	if g.Grounded && g.intent.Action == ActionCrouch {
		return PlayerHCrouch
	}
	return PlayerHStand
}

func (g *Game) Step() {
	if !g.Alive {
		return
	}
	g.Ticks++
	g.applyIntent()
	g.integrate()
	g.resolveGround()
	g.camera()
	g.aiEnemies()
	g.moveBullets()
	g.collideEntities()
	if !g.Alive {
		return
	}
	g.ensureWorld()
	g.recycle()
	g.scoreDistance()
	if g.PlayerY < GroundY-0.5 {
		col := int(math.Floor(g.PlayerX))
		if !g.solid[col] {
			g.kill("pit")
		}
	}
}

func (g *Game) applyIntent() {
	move := g.intent.Move
	action := g.intent.Action
	g.updateFacing(move)

	// Duck under bullets — slow crawl, no jump.
	if g.Grounded && action == ActionCrouch {
		switch move {
		case MoveLeft:
			g.VX = -MoveSpeedCrouch
		case MoveRight:
			g.VX = MoveSpeedCrouch
		default:
			g.VX = 0
		}
		return
	}

	if !g.Grounded {
		// Preserve takeoff momentum; only steer toward AirControl if slower/opposite.
		switch move {
		case MoveLeft:
			if g.VX > -AirControl {
				g.VX = -AirControl
			}
		case MoveRight:
			if g.VX < AirControl {
				g.VX = AirControl
			}
		default:
			g.VX *= 0.98
		}
		return
	}

	switch move {
	case MoveLeft:
		g.VX = -MoveSpeed
	case MoveRight:
		g.VX = MoveSpeed
	default:
		g.VX = 0
	}

	if action == ActionJump {
		g.takeoff(JumpV)
		return
	}
}

func (g *Game) updateFacing(move MoveIntent) {
	switch move {
	case MoveLeft:
		g.Facing = -1
	case MoveRight:
		g.Facing = 1
	}
}

// takeoff follows the current horizontal intent: directional jumps carry horizontally,
// while an IDLE jump is vertical regardless of the last facing direction.
func (g *Game) takeoff(vy float64) {
	g.VY = vy
	g.Grounded = false
	g.intent.Action = ActionNone
	switch g.intent.Move {
	case MoveLeft:
		g.VX = -MoveSpeed
	case MoveRight:
		g.VX = MoveSpeed
	default:
		g.VX = 0
	}
}

func (g *Game) integrate() {
	if !g.Grounded {
		g.VY -= Gravity * DT
	}
	g.PlayerX += g.VX * DT
	g.PlayerY += g.VY * DT

	// camera left clamp
	halfW := PlayerW / 2
	if g.PlayerX-halfW < g.CameraX {
		g.PlayerX = g.CameraX + halfW
		if g.VX < 0 {
			g.VX = 0
		}
	}
}

func (g *Game) resolveGround() {
	col := int(math.Floor(g.PlayerX))
	onSolid := g.solid[col]
	if g.VY <= 0 && g.PlayerY <= GroundY && onSolid {
		g.PlayerY = GroundY
		g.VY = 0
		g.Grounded = true
	} else if g.PlayerY > GroundY || !onSolid {
		g.Grounded = false
	}
}

func (g *Game) camera() {
	lead := g.CameraX + ViewW*CameraLead
	if g.PlayerX > lead {
		g.CameraX += g.PlayerX - lead
	}
}

func (g *Game) aiEnemies() {
	// Freeze patrol until the run starts so a random opening enemy cannot walk into an idle spawn.
	if !g.runStarted() {
		return
	}
	for i := range g.enemies {
		e := &g.enemies[i]
		if !e.Alive {
			continue
		}
		if e.Dir == 0 {
			e.Dir = -1 // default walk left (into approaching runner)
		}
		nextX := e.X + e.Dir*EnemySpeed*DT
		foot := int(math.Floor(nextX))
		// Turn around at pits / map edge — do not chase the player.
		if foot < 0 || !g.solid[foot] {
			e.Dir = -e.Dir
			continue
		}
		e.X = nextX
	}
}

func (g *Game) spawnEnemy(x float64) {
	dir := 1.0
	if g.rng.Intn(2) == 0 {
		dir = -1
	}
	g.enemies = append(g.enemies, Enemy{X: x, Y: GroundY, Dir: dir, Alive: true})
}

func (g *Game) moveBullets() {
	if !g.bulletsLive() {
		return
	}
	for i := range g.bullets {
		// Stay parked far ahead until near the right edge of the view — prevents idle snipes
		// while still letting ensureWorld place bullets into future chunks.
		if g.bullets[i].X > g.CameraX+ViewW+3 {
			continue
		}
		g.bullets[i].X -= BulletSpeed * DT
	}
}

func (g *Game) bulletsLive() bool {
	return g.runStarted()
}

// runStarted is true once the player has begun advancing (camera or position).
// Until then, far-spawned bullets stay parked and enemies do not patrol into spawn.
func (g *Game) runStarted() bool {
	return g.CameraX > 0.5 || g.PlayerX > 8
}

func (g *Game) collideEntities() {
	pw, ph := PlayerW, g.playerH()
	pl, pr := g.PlayerX-pw/2, g.PlayerX+pw/2
	pb, pt := g.PlayerY, g.PlayerY+ph

	for i := range g.enemies {
		e := &g.enemies[i]
		if !e.Alive {
			continue
		}
		el, er := e.X-EnemyW/2, e.X+EnemyW/2
		eb, et := e.Y, e.Y+EnemyH
		if pr > el && pl < er && pb < et && pt > eb {
			// Stomp if feet are clearly on/above the upper body.
			// Allow slight rise-phase contact so a mistimed jump still boots the head,
			// instead of dying to the "face" when closing speed is high.
			onTop := pb >= e.Y+EnemyH*0.45
			fallingOnHead := g.VY <= 0 && pb >= et-0.45 && pb <= et+0.20
			if onTop || fallingOnHead {
				e.Alive = false
				g.Score += 100
				g.VY = StompBounceV
				g.Grounded = false
			} else {
				g.kill("enemy")
				return
			}
		}
	}

	for _, b := range g.bullets {
		if !g.bulletsLive() {
			break
		}
		// Ignore bullets that have not entered the view yet (still off the right edge).
		if b.X > g.CameraX+ViewW {
			continue
		}
		// Bullet X is the center. Skip once the whole core has passed the player
		// so a long visual trail / late overlap with the rear edge cannot kill.
		bl, br := b.X-BulletW/2, b.X+BulletW/2
		if br < pl {
			continue
		}
		bb, bt := b.Y, b.Y+BulletH
		if pr > bl && pl < br && pb < bt && pt > bb {
			g.kill("bullet")
			return
		}
	}

	for i := range g.boxes {
		bx := &g.boxes[i]
		bl, br := bx.X-BoxW/2, bx.X+BoxW/2
		bb, bt := bx.Y, bx.Y+BoxH
		head := g.PlayerY + ph
		// Bonk from below while jumping up (or at apex). First hit scores.
		if pr > bl && pl < br && g.VY >= 0 && head >= bb-0.05 && head <= bb+0.45 && pb < bt {
			if !bx.Hit {
				bx.Hit = true
				g.Score += BoxHitScore
			}
			g.VY = 0
			g.PlayerY = bb - ph
		}
	}

	// Ground items (keys): only collectible while crouching.
	crouching := g.Grounded && g.intent.Action == ActionCrouch
	if crouching {
		for i := range g.items {
			it := &g.items[i]
			if !it.Alive {
				continue
			}
			il, ir := it.X-ItemW/2, it.X+ItemW/2
			ib, itop := it.Y, it.Y+ItemH
			if pr > il && pl < ir && pb < itop && pt > ib {
				it.Alive = false
				g.Score += ItemScore
			}
		}
	}
}

func (g *Game) kill(reason string) {
	g.Alive = false
	g.DeathReason = reason
}

func (g *Game) ensureWorld() {
	need := int(g.CameraX + ViewW + Lookahead)
	for g.genRight < need+ChunkTiles {
		g.generateChunk()
	}
}

func (g *Game) recycle() {
	limit := g.CameraX - 2
	outE := g.enemies[:0]
	for _, e := range g.enemies {
		if e.X+EnemyW/2 >= limit && e.Alive {
			outE = append(outE, e)
		}
	}
	g.enemies = outE
	outB := g.bullets[:0]
	for _, b := range g.bullets {
		if b.X+BulletW/2 >= limit {
			outB = append(outB, b)
		}
	}
	g.bullets = outB
	outX := g.boxes[:0]
	for _, b := range g.boxes {
		if b.X+BoxW/2 >= limit {
			outX = append(outX, b)
		}
	}
	g.boxes = outX
	outI := g.items[:0]
	for _, it := range g.items {
		if it.Alive && it.X+ItemW/2 >= limit {
			outI = append(outI, it)
		}
	}
	g.items = outI
}

func (g *Game) scoreDistance() {
	chunk := int(g.CameraX) / ChunkTiles
	if chunk > g.lastChunkScore {
		g.Score += 10 * (chunk - g.lastChunkScore)
		g.lastChunkScore = chunk
	}
}

func (g *Game) generateChunk() {
	start := g.genRight
	end := start + ChunkTiles

	// default solid
	for x := start; x < end; x++ {
		g.solid[x] = true
	}

	// First chunk is a flat runway; after that, seeded random hazards.
	safeIntro := start < ChunkTiles
	forceHazard := !safeIntro && g.emptyStreak >= 2

	roll := g.rng.Intn(100)
	kind := "empty"
	switch {
	case safeIntro:
		kind = "empty"
	case forceHazard:
		switch {
		case roll < 26:
			kind = "pit1"
		case roll < 34:
			kind = "twin_pit1"
		case roll < 52:
			kind = "enemy"
		case roll < 68:
			kind = "bullet"
		case roll < 82:
			kind = "item"
		default:
			kind = "box"
		}
	default:
		switch {
		case roll < 18:
			kind = "empty"
		case roll < 36:
			kind = "pit1"
		case roll < 44:
			kind = "twin_pit1"
		case roll < 60:
			kind = "enemy"
		case roll < 74:
			kind = "bullet"
		case roll < 88:
			kind = "item"
		default:
			kind = "box"
		}
	}

	switch kind {
	case "empty":
		g.emptyStreak++
	case "pit1":
		g.digPit(start+6, 1)
		g.emptyStreak = 0
	case "twin_pit1":
		g.digPit(start+4, 1)
		g.digPit(start+7, 1)
		g.emptyStreak = 0
	case "enemy":
		ex := float64(start + 10)
		aliveN := 0
		for _, e := range g.enemies {
			if e.Alive {
				aliveN++
			}
		}
		if aliveN >= 2 || ex-g.PlayerX < 8 {
			g.emptyStreak++
			break
		}
		g.spawnEnemy(ex)
		g.emptyStreak = 0
	case "bullet":
		if len(g.bullets) >= 2 {
			g.emptyStreak++
			break
		}
		bx := float64(start + 12)
		if bx < g.PlayerX+12 {
			bx = g.PlayerX + 12
		}
		minEdge := g.CameraX + ViewW + 2
		if bx < minEdge {
			bx = minEdge
		}
		g.bullets = append(g.bullets, Bullet{X: bx, Y: BulletY})
		g.emptyStreak = 0
	case "item":
		aliveN := 0
		for _, it := range g.items {
			if it.Alive {
				aliveN++
			}
		}
		if aliveN >= 3 {
			g.emptyStreak++
			break
		}
		ix := float64(start + 7)
		// Keep on solid ground.
		if !g.solid[int(ix)] {
			ix = float64(start + 9)
		}
		g.items = append(g.items, Item{X: ix, Y: GroundY, Alive: true})
		g.emptyStreak = 0
	case "box":
		if len(g.boxes) < 3 {
			g.boxes = append(g.boxes, Box{X: float64(start + 8), Y: BoxBottom, Hit: false})
			g.emptyStreak = 0
		} else {
			g.emptyStreak++
		}
	}
	g.genRight = end
}

func (g *Game) digPit(start, width int) {
	if width < 1 {
		width = 1
	}
	if width > 1 {
		width = 1 // single jump only clears 1-tile gaps
	}
	for i := 0; i < width; i++ {
		g.solid[start+i] = false
	}
}

func (g *Game) Snapshot() Snapshot {
	cam0 := int(math.Floor(g.CameraX))
	cam1 := int(math.Ceil(g.CameraX + ViewW))
	solids := make([]int, 0, cam1-cam0+1)
	pits := make([][2]float64, 0)
	inPit := false
	pitStart := 0.0
	for x := cam0; x <= cam1; x++ {
		if g.solid[x] {
			solids = append(solids, x)
			if inPit {
				pits = append(pits, [2]float64{pitStart, float64(x)})
				inPit = false
			}
		} else {
			if !inPit {
				inPit = true
				pitStart = float64(x)
			}
		}
	}
	if inPit {
		pits = append(pits, [2]float64{pitStart, float64(cam1 + 1)})
	}

	es := make([]EnemySnap, 0, len(g.enemies))
	for _, e := range g.enemies {
		if e.X > g.CameraX-1 && e.X < g.CameraX+ViewW+4 {
			face := e.Dir
			if face == 0 {
				face = -1
			}
			es = append(es, EnemySnap{X: e.X, Y: e.Y, Facing: face, Alive: e.Alive})
		}
	}
	bs := make([]BulletSnap, 0, len(g.bullets))
	for _, b := range g.bullets {
		bs = append(bs, BulletSnap{X: b.X, Y: b.Y})
	}
	xs := make([]BoxSnap, 0, len(g.boxes))
	for _, b := range g.boxes {
		xs = append(xs, BoxSnap{X: b.X, Y: b.Y, Hit: b.Hit})
	}
	is := make([]ItemSnap, 0, len(g.items))
	for _, it := range g.items {
		if it.Alive && it.X > g.CameraX-1 && it.X < g.CameraX+ViewW+4 {
			is = append(is, ItemSnap{X: it.X, Y: it.Y, Alive: true})
		}
	}

	return Snapshot{
		Seed: g.Seed, CameraX: g.CameraX, ViewW: ViewW, ViewH: ViewH, TilePx: TilePx, GroundY: GroundY,
		PlayerX: g.PlayerX, PlayerY: g.PlayerY, PlayerVX: g.VX, PlayerVY: g.VY,
		PlayerW: PlayerW, PlayerH: g.playerH(), Facing: g.Facing,
		Crouch: g.Grounded && g.intent.Action == ActionCrouch,
		Grounded: g.Grounded, Alive: g.Alive, Score: g.Score,
		Distance: int(math.Floor(g.CameraX)), Ticks: g.Ticks, DeathReason: g.DeathReason,
		Move: string(g.intent.Move), Action: string(g.intent.Action),
		Solids: solids, Pits: pits, Enemies: es, Bullets: bs, Boxes: xs, Items: is,
	}
}

// aheadBox returns center-to-center dx to the nearest unhit overhead box
// (behind allowed so HazardSummary can still report a just-passed box).
func (g *Game) aheadBox() float64 {
	best := 99.0
	for _, b := range g.boxes {
		if b.Hit {
			continue
		}
		dx := b.X - g.PlayerX
		if dx >= BoxSeekMin && dx < best {
			best = dx
		}
	}
	return best
}

func (g *Game) HazardSummary() string {
	return g.DecisionCues().Text
}

// DecisionCues is the typed platform judgment (when to walk/jump/crouch).
// Far pits/enemies/bullets are intentionally ignored: they must not freeze the runner.
type DecisionCues struct {
	Need         string // NONE | JUMP | CROUCH
	Go           string // RIGHT | IDLE
	LockMoveToGo bool   // enforce the safe strategy direction (forward or idle)
	Text         string
}

func (g *Game) DecisionCues() DecisionCues {
	type hz struct {
		kind string
		dx   float64
	}
	var hs []hz
	for x := int(g.PlayerX); x < int(g.PlayerX)+12; x++ {
		if !g.solid[x] {
			w := 1
			for g.solid[x+w] == false && w < 3 {
				w++
			}
			if w > 2 {
				w = 2
			}
			hs = append(hs, hz{fmt.Sprintf("pit%d", w), float64(x) - g.PlayerX})
			break
		}
	}
	for _, e := range g.enemies {
		if e.Alive && e.X >= g.PlayerX-0.5 {
			hs = append(hs, hz{"enemy", e.X - g.PlayerX})
			break
		}
	}
	for _, b := range g.bullets {
		if b.X >= g.PlayerX-1 && b.X <= g.CameraX+ViewW+6 {
			hs = append(hs, hz{"bullet", b.X - g.PlayerX})
			break
		}
	}
	if dx := g.aheadBox(); dx < 99 {
		hs = append(hs, hz{"box", dx})
	}
	for _, it := range g.items {
		if it.Alive && it.X >= g.PlayerX-0.5 {
			hs = append(hs, hz{"item", it.X - g.PlayerX})
			break
		}
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].dx < hs[j].dx })

	var bulletDX, pitDX, enemyDX, boxDX, itemDX float64 = 99, 99, 99, 99, 99
	var pitKind string
	for _, h := range hs {
		switch {
		case h.kind == "bullet" && h.dx < bulletDX:
			bulletDX = h.dx
		case strings.HasPrefix(h.kind, "pit") && h.dx < pitDX:
			pitDX, pitKind = h.dx, h.kind
		case h.kind == "enemy" && h.dx < enemyDX:
			enemyDX = h.dx
		case h.kind == "box" && h.dx < boxDX:
			boxDX = h.dx
		case h.kind == "item" && h.dx < itemDX:
			itemDX = h.dx
		}
	}
	bulletNear := bulletDX >= BulletCrouchMin && bulletDX < BulletCrouchMax
	pitNear := pitKind != "" && pitDX >= 0 && pitDX < 2.0
	pitUrgent := pitNear && pitDX < 1.1 && (!bulletNear || pitDX < bulletDX)
	enemyStomp := enemyDX >= EnemyStompMin && enemyDX <= EnemyStompMax
	enemyTooClose := enemyDX >= -0.2 && enemyDX < EnemyStompMin
	enemyEngage := enemyDX >= -0.2 && enemyDX <= EnemyStompArm
	boxReady := boxDX >= BoxBonkMin && boxDX <= BoxBonkMax
	itemTarget := !bulletNear && !enemyEngage && itemDX >= ItemEatMin && itemDX <= ItemApproachMax
	itemReady := itemTarget && itemDX <= ItemEatMax
	itemStop := itemTarget && itemDX <= ItemStopMax

	need := "NONE"
	switch {
	case !g.Grounded:
		need = "NONE"
	case bulletNear && !pitUrgent:
		need = "CROUCH"
	case pitNear || enemyStomp || enemyTooClose || (boxReady && !bulletNear):
		need = "JUMP"
	case itemReady:
		need = "CROUCH"
	}
	goDir := "RIGHT"
	if enemyTooClose || itemStop {
		goDir = "IDLE"
	}
	lockMoveToGo := itemTarget || enemyTooClose

	parts := []string{
		"need=" + need,
		"go=" + goDir,
		fmt.Sprintf("ground=%d", boolInt(g.Grounded)),
	}
	const listMaxDX = 6.0
	listed := 0
	for i := 0; i < len(hs) && listed < 3; i++ {
		if hs[i].dx > listMaxDX {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s@%.1f", hs[i].kind, hs[i].dx))
		listed++
	}
	return DecisionCues{
		Need:         need,
		Go:           goDir,
		LockMoveToGo: lockMoveToGo,
		Text:         fmt.Sprintf("%s x=%.1f", joinSpace(parts), g.PlayerX-g.CameraX),
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func joinSpace(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
