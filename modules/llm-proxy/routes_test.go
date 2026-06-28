package main

import "testing"

func TestBuildRoutes_AddsInteractiveVariants(t *testing.T) {
	specs := []routeSpec{
		{prefix: "/anthropic", upstream: "https://api.anthropic.com", apiKey: "k", provider: "anthropic", requiresKey: true},
		{prefix: "/openai", upstream: "https://api.openai.com", apiKey: "k", provider: "openai", requiresKey: true},
	}
	routes, err := buildRoutes(specs)
	if err != nil {
		t.Fatalf("buildRoutes: %v", err)
	}
	// Every spec yields a fleet route AND an interactive route.
	if len(routes) != 4 {
		t.Fatalf("len(routes)=%d want 4", len(routes))
	}
	byPrefix := map[string]route{}
	for _, r := range routes {
		byPrefix[r.prefix] = r
	}
	if byPrefix["/anthropic"].class != classFleet {
		t.Fatalf("/anthropic class=%q want fleet", byPrefix["/anthropic"].class)
	}
	ia, ok := byPrefix["/interactive/anthropic"]
	if !ok {
		t.Fatal("missing /interactive/anthropic route")
	}
	if ia.class != classInteractive {
		t.Fatalf("/interactive/anthropic class=%q want interactive", ia.class)
	}
	if ia.provider != "anthropic" {
		t.Fatalf("interactive provider=%q want anthropic", ia.provider)
	}
}
