package loadregions

import (
	"strings"
	"testing"
)

func TestDetectCycles_NoCycle(t *testing.T) {
	f := File{Regions: []Region{
		{Slug: "ny", ScopeTier: "regional", Kind: "us:state", Name: "NY", Parents: nil},
		{Slug: "nyc-metro", ScopeTier: "regional", Kind: "us:metro", Name: "NYC Metro", Parents: []string{"ny"}},
		{Slug: "brooklyn", ScopeTier: "local", Kind: "us:borough", Name: "Brooklyn", Parents: []string{"nyc-metro"}},
	}}
	if err := DetectCycles(f); err != nil {
		t.Errorf("DetectCycles: %v", err)
	}
}

func TestDetectCycles_DirectCycle(t *testing.T) {
	f := File{Regions: []Region{
		{Slug: "a", ScopeTier: "local", Kind: "x", Name: "A", Parents: []string{"b"}},
		{Slug: "b", ScopeTier: "local", Kind: "x", Name: "B", Parents: []string{"a"}},
	}}
	err := DetectCycles(f)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err lacks 'cycle': %v", err)
	}
}

func TestDetectCycles_LongCycle(t *testing.T) {
	f := File{Regions: []Region{
		{Slug: "a", ScopeTier: "local", Kind: "x", Name: "A", Parents: []string{"b"}},
		{Slug: "b", ScopeTier: "local", Kind: "x", Name: "B", Parents: []string{"c"}},
		{Slug: "c", ScopeTier: "local", Kind: "x", Name: "C", Parents: []string{"a"}},
	}}
	if err := DetectCycles(f); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestDetectCycles_UnknownParentSlug(t *testing.T) {
	f := File{Regions: []Region{
		{Slug: "a", ScopeTier: "local", Kind: "x", Name: "A", Parents: []string{"ghost"}},
	}}
	err := DetectCycles(f)
	if err == nil {
		t.Fatal("expected unknown-parent error")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err should name the missing slug: %v", err)
	}
}
