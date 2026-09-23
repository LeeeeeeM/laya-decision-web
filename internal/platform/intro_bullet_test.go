package platform

import "testing"

func TestClearFirstPitWhenPresent(t *testing.T) {
	// Walk right and jump the first pit if any. Surviving past spawn is enough;
	// later random hazards may still kill — that is OK.
	g := NewGame(7)
	g.SetIntent(MoveRight, ActionNone)
	jumped := false
	for i := 0; i < 60*5; i++ {
		col := int(g.PlayerX + 1.2)
		if !jumped && g.Grounded && col > 0 && !g.solid[col] {
			g.SetIntent(MoveRight, ActionJump)
			jumped = true
		} else if jumped && g.Grounded {
			g.SetIntent(MoveRight, ActionNone)
			jumped = false
		}
		g.Step()
		if g.PlayerX > 16 {
			return
		}
		if !g.Alive && g.DeathReason == "bullet" && g.PlayerX < 12 {
			t.Fatalf("parked bullet sniped too early: x=%.2f", g.PlayerX)
		}
	}
}
