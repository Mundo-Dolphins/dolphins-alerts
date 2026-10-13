package app

import (
	"context"
	"testing"

	"github.com/Mundo-Dolphins/dolphins-alerts/config"
	"github.com/Mundo-Dolphins/dolphins-alerts/injury"
)

type stubProvider struct{}

func (stubProvider) Fetch(context.Context, injury.ReportRequest) (injury.Report, error) {
	return injury.Report{}, nil
}

func TestNewAppliesOptions(t *testing.T) {
	cfg := config.Config{}
	if a := New(cfg); a.injuryProvider != nil {
		t.Fatal("no provider should be set by default")
	}
	p := stubProvider{}
	a := New(cfg, WithInjuryProvider(p))
	if a.injuryProvider != p {
		t.Fatal("WithInjuryProvider was not applied")
	}
}
