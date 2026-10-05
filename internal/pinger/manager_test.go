package pinger

import "testing"

func TestReorderByIndices(t *testing.T) {
	m := NewManager(0)
	m.Add("a", "")
	m.Add("b", "")
	m.Add("c", "")
	m.ReorderByIndices([]int{2}, 0) // c to front
	hosts := m.TargetsForConfig()
	if hosts[0].Host != "c" || hosts[1].Host != "a" || hosts[2].Host != "b" {
		t.Fatalf("got %#v", hosts)
	}
	m.ReorderByIndices([]int{0}, 3) // c to end
	hosts = m.TargetsForConfig()
	if hosts[0].Host != "a" || hosts[1].Host != "b" || hosts[2].Host != "c" {
		t.Fatalf("got %#v", hosts)
	}
}

func TestMoveUpDown(t *testing.T) {
	m := NewManager(0)
	idA := m.Add("a", "")
	idB := m.Add("b", "")
	_ = m.Add("c", "")
	if !m.MoveDown(idA) {
		t.Fatal("MoveDown a")
	}
	hosts := m.TargetsForConfig()
	if hosts[0].Host != "b" || hosts[1].Host != "a" || hosts[2].Host != "c" {
		t.Fatalf("after MoveDown: %#v", hosts)
	}
	if m.MoveUp(idB) {
		t.Fatal("MoveUp b at top should fail")
	}
	if !m.MoveUp(idA) {
		t.Fatal("MoveUp a")
	}
	hosts = m.TargetsForConfig()
	if hosts[0].Host != "a" || hosts[1].Host != "b" || hosts[2].Host != "c" {
		t.Fatalf("after MoveUp: %#v", hosts)
	}
}
