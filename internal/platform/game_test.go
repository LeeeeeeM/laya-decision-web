package platform

import "testing"

func TestJumpClearsOneTilePit(t *testing.T) {
	g := blankGame()
	g.digPit(8, 1)
	g.PlayerX = 7.0
	g.SetIntent(MoveRight, ActionJump)
	for i := 0; i < 120; i++ {
		g.Step()
		if !g.Alive {
			t.Fatalf("died: %s at x=%.2f", g.DeathReason, g.PlayerX)
		}
		if g.Grounded && g.PlayerX > 9 {
			return
		}
	}
	t.Fatalf("did not clear pit, x=%.2f grounded=%v", g.PlayerX, g.Grounded)
}

func TestJumpUsesFacingWhenIdle(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.SetIntent(MoveLeft, ActionNone)
	g.Step()
	if g.Facing != -1 {
		t.Fatalf("expected face left, got %d", g.Facing)
	}
	g.SetIntent(MoveIdle, ActionJump)
	g.Step()
	if g.VX >= 0 {
		t.Fatalf("idle jump should keep facing left, vx=%.2f facing=%d", g.VX, g.Facing)
	}
	g.SetIntent(MoveRight, ActionNone)
	for i := 0; i < 30; i++ {
		g.Step()
	}
	g.SetIntent(MoveIdle, ActionJump)
	g.Step()
	if g.VX <= 0 {
		t.Fatalf("idle jump should face right, vx=%.2f facing=%d", g.VX, g.Facing)
	}
}

func TestCrouchDodgesBullet(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 1
	g.bullets = []Bullet{{X: g.PlayerX + 2, Y: BulletY}}
	g.SetIntent(MoveIdle, ActionCrouch)
	for i := 0; i < 90; i++ {
		g.Step()
		if !g.Alive {
			t.Fatalf("crouch should dodge bullet, died reason=%s tick=%d h=%.2f", g.DeathReason, g.Ticks, g.playerH())
		}
		if len(g.bullets) == 0 {
			break
		}
	}
	if !g.Alive {
		t.Fatal("expected alive after crouch dodge")
	}
	if !g.Snapshot().Crouch {
		t.Fatal("expected crouch pose while holding CROUCH")
	}
}

func TestStandHitByBullet(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 1
	g.bullets = []Bullet{{X: g.PlayerX, Y: BulletY}}
	g.SetIntent(MoveIdle, ActionNone)
	hit := false
	for i := 0; i < 30; i++ {
		g.bullets[0].X = g.PlayerX
		g.Step()
		if !g.Alive && g.DeathReason == "bullet" {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("standing player should be hit by bullet")
	}
}

func TestBulletTailDoesNotKill(t *testing.T) {
	// Bullet center already past the player; only an imaginary long tail would overlap.
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 1
	pw := PlayerW
	pl := g.PlayerX - pw/2
	g.bullets = []Bullet{{X: pl - BulletW - 0.05, Y: BulletY}}
	g.SetIntent(MoveIdle, ActionNone)
	for i := 0; i < 5; i++ {
		g.Step()
		if !g.Alive {
			t.Fatalf("passed bullet tail should not kill, reason=%s", g.DeathReason)
		}
	}
}

func TestCrouchPicksUpItem(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.items = []Item{{X: 10.2, Y: GroundY, Alive: true}}
	g.SetIntent(MoveIdle, ActionNone)
	g.Step()
	if len(g.items) != 1 || !g.items[0].Alive {
		t.Fatal("standing should not pick up item")
	}
	g.SetIntent(MoveIdle, ActionCrouch)
	for i := 0; i < 5; i++ {
		g.Step()
	}
	for _, it := range g.items {
		if it.Alive {
			t.Fatal("crouch overlap should pick up item")
		}
	}
	if g.Score < ItemScore {
		t.Fatalf("expected +%d score, got %d", ItemScore, g.Score)
	}
}

func TestWalkOverItemDoesNotPickup(t *testing.T) {
	g := blankGame()
	g.PlayerX = 9
	g.items = []Item{{X: 10, Y: GroundY, Alive: true}}
	g.SetIntent(MoveRight, ActionNone)
	for i := 0; i < 40; i++ {
		g.Step()
	}
	alive := false
	for _, it := range g.items {
		if it.Alive {
			alive = true
		}
	}
	if !alive {
		t.Fatal("walking upright should leave item on ground")
	}
}

func TestJumpHitsBoxForScore(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.boxes = []Box{{X: 10.0, Y: BoxBottom, Hit: false}}
	g.SetIntent(MoveIdle, ActionJump)
	hit := false
	for i := 0; i < 90; i++ {
		g.Step()
		if g.boxes[0].Hit {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatalf("expected jump to bonk box, feet=%.2f head=%.2f boxBottom=%.2f", g.PlayerY, g.PlayerY+g.playerH(), BoxBottom)
	}
	if g.Score < BoxHitScore {
		t.Fatalf("expected +%d for box hit, got %d", BoxHitScore, g.Score)
	}
}

func TestEnemyPatrolTurnsAtPit(t *testing.T) {
	g := blankGame()
	g.digPit(14, 1)
	g.PlayerX = 2 // stay clear of stomp/collision
	g.CameraX = 1 // runStarted so patrol AI is live
	g.SetIntent(MoveIdle, ActionNone)
	g.enemies = []Enemy{{X: 12.5, Y: GroundY, Dir: 1, Alive: true}}
	flipped := false
	for i := 0; i < 180; i++ {
		g.Step()
		if len(g.enemies) == 0 {
			t.Fatal("enemy disappeared")
		}
		if g.enemies[0].Dir < 0 {
			flipped = true
			break
		}
	}
	if !flipped {
		t.Fatalf("expected patrol to turn at pit, x=%.2f dir=%.0f", g.enemies[0].X, g.enemies[0].Dir)
	}
	xAtFlip := g.enemies[0].X
	for i := 0; i < 60; i++ {
		g.Step()
	}
	if g.enemies[0].X >= 14 {
		t.Fatalf("enemy walked into pit zone, x=%.2f (flip at %.2f)", g.enemies[0].X, xAtFlip)
	}
	if g.enemies[0].X >= xAtFlip {
		t.Fatalf("expected to walk left after flip, x=%.2f was %.2f", g.enemies[0].X, xAtFlip)
	}
}

func blankGame() *Game {
	g := NewGame(1)
	g.enemies = nil
	g.bullets = nil
	g.boxes = nil
	g.items = nil
	for x := 0; x < 80; x++ {
		g.solid[x] = true
	}
	g.genRight = 80
	g.CameraX = 0
	g.PlayerX = 4
	g.PlayerY = GroundY
	g.Grounded = true
	g.Alive = true
	g.VX, g.VY = 0, 0
	g.Facing = 1
	return g
}
