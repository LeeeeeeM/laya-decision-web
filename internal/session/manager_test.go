package session_test

import (
	"testing"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
	"github.com/LeeeeeeM/laya-decision-web/internal/decision/mockprovider"
	"github.com/LeeeeeeM/laya-decision-web/internal/session"
)

func TestSessionCreatePauseResumeResetStop(t *testing.T) {
	mgr := session.NewManager(map[string]decision.Provider{
		decision.ProviderMock: mockprovider.New(),
	}, 2)
	fps := 20.0
	guarded := true
	s, err := mgr.Create(session.CreateRequest{
		Provider: decision.ProviderMock,
		FPS:      fps,
		Seed:     7,
		Width:    24,
		Height:   16,
		Guarded:  &guarded,
		Prompt:   "compact",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Delete(s.ID)

	ch, unsub := s.Subscribe(32)
	defer unsub()

	// Wait for at least one decision/snapshot after create.
	deadline := time.After(3 * time.Second)
	sawDecision := false
	for !sawDecision {
		select {
		case ev := <-ch:
			if ev.Type == "decision" {
				sawDecision = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for decision")
		}
	}

	if _, err := s.Control(session.ControlRequest{Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Status != session.StatusPaused {
		t.Fatalf("status=%s", s.Snapshot().Status)
	}
	if _, err := s.Control(session.ControlRequest{Action: "resume"}); err != nil {
		t.Fatal(err)
	}
	seed := int64(9)
	if _, err := s.Control(session.ControlRequest{Action: "reset", Seed: &seed}); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Game.Seed != 9 || s.Snapshot().Game.Ticks != 0 {
		t.Fatalf("%+v", s.Snapshot().Game)
	}
	if _, err := s.Control(session.ControlRequest{Action: "stop"}); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Status != session.StatusStopped {
		t.Fatalf("status=%s", s.Snapshot().Status)
	}
}

func TestSessionRejectsUnavailableProvider(t *testing.T) {
	mgr := session.NewManager(map[string]decision.Provider{
		decision.ProviderMock: mockprovider.New(),
	}, 2)
	_, err := mgr.Create(session.CreateRequest{Provider: "missing", FPS: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCapabilitiesListsMock(t *testing.T) {
	mgr := session.NewManager(map[string]decision.Provider{
		decision.ProviderMock: mockprovider.New(),
	}, 2)
	caps := mgr.Capabilities()
	found := false
	for _, p := range caps.Providers {
		if p.ID == decision.ProviderMock && p.Available {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", caps)
	}
}
