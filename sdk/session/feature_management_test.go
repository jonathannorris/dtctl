package session

import "testing"

func TestSetContextWithOptions_FMProject(t *testing.T) {
	str := func(s string) *string { return &s }
	c := NewConfig()

	c.SetContextWithOptions("dev", "https://dev.example.invalid", "dev-token", &ContextOptions{FMProject: str("checkout")})
	c.CurrentContext = "dev"
	if got := c.FMProject(); got != "checkout" {
		t.Fatalf("FMProject after create = %q, want checkout", got)
	}

	// An update that does not mention the project keeps it.
	c.SetContextWithOptions("dev", "https://dev.example.invalid", "", &ContextOptions{Description: "x"})
	if got := c.FMProject(); got != "checkout" {
		t.Fatalf("FMProject after unrelated update = %q, want checkout", got)
	}

	// An empty value clears it and drops the now-empty block.
	c.SetContextWithOptions("dev", "https://dev.example.invalid", "", &ContextOptions{FMProject: str("")})
	if got := c.FMProject(); got != "" {
		t.Fatalf("FMProject after clear = %q, want empty", got)
	}
	if c.Contexts[0].Context.FeatureManagement != nil {
		t.Error("clearing the only FM default must drop the feature-management block")
	}
}
