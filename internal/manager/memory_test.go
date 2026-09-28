package manager

import (
	"reflect"
	"testing"
)

func TestRecommendedMemoryGB(t *testing.T) {
	tests := []struct{ total, want int }{{2, 1}, {4, 2}, {8, 4}, {16, 6}, {32, 8}}
	for _, test := range tests {
		if got := RecommendedMemoryGB(test.total); got != test.want {
			t.Errorf("RecommendedMemoryGB(%d) = %d, want %d", test.total, got, test.want)
		}
	}
}

func TestBuildMemoryPlanAutomatic(t *testing.T) {
	plan, err := BuildMemoryPlan(16, MemoryModeAutomatic, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaximumGB != 6 || plan.InitialGB != 2 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if got, want := plan.JVMArgs(), []string{"-Xms2G", "-Xmx6G"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("JVMArgs = %v, want %v", got, want)
	}
}

func TestBuildMemoryPlanManual(t *testing.T) {
	plan, err := BuildMemoryPlan(12, MemoryModeManual, 5)
	if err != nil || plan.MaximumGB != 5 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if _, err := BuildMemoryPlan(8, MemoryModeManual, 7); err == nil {
		t.Fatal("expected validation error when manual heap does not preserve system RAM")
	}
	for _, invalid := range []int{0, -1} {
		if _, err := BuildMemoryPlan(8, MemoryModeManual, invalid); err == nil {
			t.Fatalf("expected manual heap %d to be rejected", invalid)
		}
	}
	if _, err := BuildMemoryPlan(8, "invalid", 2); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
