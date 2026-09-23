package platform

import (
	"strings"
	"testing"
)

func TestHazardSummaryAirborneForcesNone(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 0
	g.Grounded = false
	g.digPit(11, 1)
	g.boxes = []Box{{X: 10 + 0.5, Y: BoxBottom, Hit: false}}
	s := g.HazardSummary()
	if !strings.Contains(s, "need=NONE") || !strings.Contains(s, "ground=0") {
		t.Fatalf("airborne must be need=NONE: %q", s)
	}
}

func TestHazardSummaryFarPitIsNone(t *testing.T) {
	g := blankGame()
	g.PlayerX = 4
	g.CameraX = 0
	g.Grounded = true
	g.digPit(22, 1)
	s := g.HazardSummary()
	if strings.Contains(s, "need=JUMP") {
		t.Fatalf("far pit must not demand JUMP: %q", s)
	}
	if strings.Contains(s, "pit") {
		t.Fatalf("far pit must not appear in state: %q", s)
	}
	if !strings.Contains(s, "go=RIGHT") {
		t.Fatalf("default go must be RIGHT: %q", s)
	}
}

func TestHazardSummaryHidesFarEnemyAndBullet(t *testing.T) {
	g := blankGame()
	g.PlayerX = 4
	g.CameraX = 0
	g.Grounded = true
	g.enemies = []Enemy{{X: 4 + 12, Y: GroundY, Alive: true}}
	g.bullets = []Bullet{{X: 4 + 20, Y: BulletY}}
	s := g.HazardSummary()
	if strings.Contains(s, "enemy@") || strings.Contains(s, "bullet@") {
		t.Fatalf("far threats must be hidden: %q", s)
	}
	if !strings.Contains(s, "need=NONE") || !strings.Contains(s, "go=RIGHT") {
		t.Fatalf("far threats must stay need=NONE go=RIGHT: %q", s)
	}
}

func TestHazardSummaryStopsAndJumpsWhenEnemyTooClose(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 0
	g.Grounded = true
	g.enemies = []Enemy{{X: 10 + 0.5, Y: GroundY, Alive: true}}
	s := g.HazardSummary()
	if !strings.Contains(s, "go=IDLE") || !strings.Contains(s, "need=JUMP") {
		t.Fatalf("close enemy must cue an idle jump, got: %q", s)
	}
}

func TestSetIntentDropsJumpInAir(t *testing.T) {
	g := blankGame()
	g.Grounded = false
	g.SetIntent(MoveRight, ActionJump)
	if g.Intent().Action != ActionNone {
		t.Fatalf("air JUMP must not queue, got %s", g.Intent().Action)
	}
}

func TestHazardSummaryNeedCrouch(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 5
	g.bullets = []Bullet{{X: 10 + 3.5, Y: BulletY}}
	s := g.HazardSummary()
	if !strings.Contains(s, "need=CROUCH") || !strings.Contains(s, "bullet@") {
		t.Fatalf("summary should demand crouch: %q", s)
	}
}

func TestHazardSummaryNoCrouchForDistantBullet(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 5
	g.Grounded = true
	g.bullets = []Bullet{{X: 10 + 9.0, Y: BulletY}}
	s := g.HazardSummary()
	if strings.Contains(s, "need=CROUCH") {
		t.Fatalf("distant bullet must not set need=CROUCH: %q", s)
	}
}

func TestHazardSummaryNeedJumpForBox(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 0
	g.Grounded = true
	g.boxes = []Box{{X: 10 + 1.0, Y: BoxBottom, Hit: false}}
	s := g.HazardSummary()
	if !strings.Contains(s, "need=JUMP") || !strings.Contains(s, "box@") {
		t.Fatalf("summary should demand jump for box: %q", s)
	}
}

func TestHazardSummaryPrefersBulletOverBox(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 5
	g.Grounded = true
	g.boxes = []Box{{X: 10 + 0.8, Y: BoxBottom, Hit: false}}
	g.bullets = []Bullet{{X: 10 + 3.0, Y: BulletY}}
	s := g.HazardSummary()
	if !strings.Contains(s, "need=CROUCH") {
		t.Fatalf("survival bullet should beat nearer box: %q", s)
	}
}

func TestGameDoesNotAutoJumpWithoutIntent(t *testing.T) {
	g := blankGame()
	g.PlayerX = 10
	g.CameraX = 0
	g.Grounded = true
	g.boxes = []Box{{X: 10 + 1.0, Y: BoxBottom, Hit: false}}
	g.enemies = []Enemy{{X: 10 + 1.6, Y: GroundY, Alive: true}}
	g.SetIntent(MoveRight, ActionNone)
	g.Step()
	if !g.Grounded || g.VY != 0 {
		t.Fatalf("game must not auto-jump; only ActionJump jumps, grounded=%v vy=%.2f", g.Grounded, g.VY)
	}
}
