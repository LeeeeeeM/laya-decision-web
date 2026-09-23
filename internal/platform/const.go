package platform

const (
	TilePx     = 32.0
	ViewW      = 24.0
	ViewH      = 14.0
	GroundY    = 2.0
	ChunkTiles = 16
	Lookahead  = 8.0

	PhysicsHz  = 60
	DT         = 1.0 / PhysicsHz
	DefaultDHz = 8.0

	MoveSpeed       = 6.0
	MoveSpeedCrouch = 3.0
	AirControl      = 4.5
	Gravity         = 40.0
	JumpV           = 10.0 // single jump — clears 1-tile pits
	StompBounceV    = 8.0

	PlayerW       = 0.75
	PlayerHStand  = 1.75
	PlayerHCrouch = 1.0

	EnemyW     = 1.0
	EnemyH     = 1.0
	EnemySpeed = 4.0
	// Stomp cue window (center-to-center). The upper bound gives the agent
	// enough decision ticks to react before reaching the enemy.
	EnemyStompMin = 1.15
	EnemyStompMax = 3.40
	EnemyStompArm = EnemyStompMax

	BulletW = 0.40 // compact core — avoid long "tail" kills after the projectile has passed
	BulletH = 0.32
	// High enough that a crouch (height 1.0) clears it; standing (1.75) still gets hit.
	BulletY     = GroundY + 1.15
	BulletSpeed = 8.0
	// Duck only when the bullet is about to arrive (~0.5s at BulletSpeed).
	// Used by HazardSummary need=CROUCH.
	BulletCrouchMin = -0.5
	BulletCrouchMax = 4.5

	BoxW      = 1.0
	BoxH      = 1.0
	// Low enough that a short jump hits the underside while still rising.
	BoxBottom   = GroundY + 2.75
	BoxHitScore = 50
	// Bonk takeoff window (center-to-center). HazardSummary need=JUMP cues.
	// BoxSeekMin: still report a just-passed unhit box in state.
	BoxBonkMin = -0.35
	BoxBonkMax = 1.80
	BoxSeekMin = -2.0

	// Ground pickup items (drawn as a key): crouch-overlap to collect.
	// Standing walk-over ignores them — forces an explicit CROUCH decision.
	ItemW      = 0.70
	ItemH      = 0.70
	ItemScore  = 40
	ItemEatMin = 0.0
	ItemEatMax = 2.00 // start the CROUCH cue before reaching the pickup

	CameraLead = 0.40
)

type MoveIntent string
type ActionIntent string

const (
	MoveIdle  MoveIntent = "IDLE"
	MoveLeft  MoveIntent = "LEFT"
	MoveRight MoveIntent = "RIGHT"

	ActionNone   ActionIntent = "NONE"
	ActionJump   ActionIntent = "JUMP"
	ActionCrouch ActionIntent = "CROUCH" // duck under bullets / pick up ground items
)
