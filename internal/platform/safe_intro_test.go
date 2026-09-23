package platform

import "testing"

func TestIdleSpawnNotSniped(t *testing.T) {
	for _, seed := range []int64{7, 1, 42, 99, 12345, 2024, 999} {
		g := NewGame(seed)
		g.SetIntent(MoveIdle, ActionNone)
		for i := 0; i < 60*20; i++ {
			g.Step()
			if !g.Alive {
				t.Fatalf("seed=%d idle death reason=%s tick=%d", seed, g.DeathReason, g.Ticks)
			}
		}
	}
}

func TestOpeningVariesBySeed(t *testing.T) {
	type sig struct {
		pits, boxes, enemies, items, bullets int
	}
	seen := map[sig]int64{}
	for _, seed := range []int64{1, 2, 3, 7, 11, 42, 99, 2024, 9999, 12345} {
		g := NewGame(seed)
		s := g.Snapshot()
		k := sig{len(s.Pits), len(s.Boxes), len(s.Enemies), len(s.Items), len(s.Bullets)}
		if prev, ok := seen[k]; ok && prev != seed {
			// Same counts can collide; also compare first hazard x if any.
			g2 := NewGame(prev)
			if sameOpeningLayout(g, g2) {
				continue // rare exact collision; not a failure by itself
			}
		}
		seen[k] = seed
	}
	if len(seen) < 3 {
		t.Fatalf("expected opening layouts to vary by seed, got %d distinct signatures: %v", len(seen), seen)
	}
}

func sameOpeningLayout(a, b *Game) bool {
	if len(a.boxes) != len(b.boxes) || len(a.enemies) != len(b.enemies) ||
		len(a.items) != len(b.items) || len(a.bullets) != len(b.bullets) {
		return false
	}
	for i := range a.boxes {
		if a.boxes[i].X != b.boxes[i].X {
			return false
		}
	}
	for i := range a.enemies {
		if a.enemies[i].X != b.enemies[i].X {
			return false
		}
	}
	for i := range a.items {
		if a.items[i].X != b.items[i].X {
			return false
		}
	}
	for i := range a.bullets {
		if a.bullets[i].X != b.bullets[i].X {
			return false
		}
	}
	// Compare solids in the opening view band.
	for x := 16; x < 40; x++ {
		if a.solid[x] != b.solid[x] {
			return false
		}
	}
	return true
}

func TestProceduralHazardsEventuallyAppear(t *testing.T) {
	g := NewGame(42)
	g.SetIntent(MoveRight, ActionNone)
	for i := 0; i < 60*15; i++ {
		g.Step()
		if !g.Alive {
			// Dying to a hazard still proves content appeared.
			return
		}
		if len(g.boxes) > 0 || len(g.enemies) > 0 || len(g.bullets) > 0 || len(g.items) > 0 {
			return
		}
		// Pits leave holes in solid.
		for x := int(g.CameraX); x < int(g.CameraX+ViewW); x++ {
			if !g.solid[x] {
				return
			}
		}
	}
	t.Fatal("expected some procedural hazard within 15s of running right")
}

func TestEarlyRunParkedBulletDoesNotSnipe(t *testing.T) {
	// Regardless of seed layout, standing still must not be hit by far-parked bullets.
	for _, seed := range []int64{7, 42, 99} {
		g := NewGame(seed)
		g.SetIntent(MoveIdle, ActionNone)
		for i := 0; i < 60*3; i++ {
			g.Step()
			if !g.Alive {
				t.Fatalf("seed=%d early idle sniped: %s", seed, g.DeathReason)
			}
		}
	}
}
