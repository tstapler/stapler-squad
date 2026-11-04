package main

import "testing"

func TestTreeRSSCountsDescendantsOnce(t *testing.T) {
	procs := []Proc{
		{1, 0, 100, "init"},
		{10, 1, 2000, "java --output_base=/ob/a server"},
		{11, 10, 300, "go compile"},
		{12, 11, 50, "asm"},
		{20, 1, 4000, "java --output_base=/ob/b server"},
		{30, 1, 999, "unrelated"},
		{40, 12, 5, "nested --output_base=/ob/a"}, // matched and already reachable from 10
	}
	kb, n := TreeRSS(procs, []string{"--output_base=/ob/a"})
	if kb != 2355 || n != 4 {
		t.Fatalf("a only: kb=%d n=%d", kb, n)
	}
	kb, _ = TreeRSS(procs, []string{"--output_base=/ob/a", "--output_base=/ob/b"})
	if kb != 6355 {
		t.Fatalf("a+b: %d", kb)
	}
	if kb, n = TreeRSS(procs, []string{""}); kb != 0 || n != 0 {
		t.Fatal("empty match must select nothing")
	}
}

func TestPeakAndMedian(t *testing.T) {
	var p Peak
	for _, v := range []int64{5, 9, 3} {
		p.Add(v)
	}
	if p.MaxKB != 9 || p.Samples != 3 {
		t.Fatal(p)
	}
	if Median([]float64{3, 1, 2}) != 2 || Median([]float64{1, 2, 3, 4}) != 2.5 || Median(nil) != -1 {
		t.Fatal("median")
	}
}

func TestParsers(t *testing.T) {
	if ParseLoad1("1.75 2 3 1/2 3") != 1.75 || ParseLoad1("x") != -1 || ParseLoad1("") != -1 {
		t.Fatal("load")
	}
	if g := ParseMemAvailGB("MemTotal: 1 kB\nMemAvailable:   2097152 kB\n"); g != 2 {
		t.Fatal(g)
	}
	if ParseMemAvailGB("nothing") != -1 {
		t.Fatal("absent")
	}
}
