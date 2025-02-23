package client

import "testing"

func TestToString(t *testing.T) {
	if ToString(1) != "1" {
		t.Error("ToString(1) failed")
	}
	if ToString("hello") != "hello" {
		t.Error("ToString(hello) failed")
	}
	if ToString(3.14) != "3.14" {
		t.Error("ToString(3.14) failed")
	}
}
