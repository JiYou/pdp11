package pdp11

import "testing"

func TestXOR(t *testing.T) {
	for _, tt := range []struct {
		x, y, z int
	}{
		{0, 0, 0},
		{1, 0, 1},
		{0, 1, 1},
		{1, 1, 0},
	} {
		got := xor(tt.x, tt.y)
		if got != tt.z {
			t.Errorf("xor(%d, %d) = %d; want %d", tt.x, tt.y, got, tt.z)
		}
	}
}

const N = 2000000

var pdpTests = []struct {
	input  string
	cycles int
}{
	{"", N},
	{"STTY -LCASE\n", N},
	{"date\n", N}, // processor loops
	// { "ls\n", N }, 	// read from odd address
	//{"who\n", N}, // read from no-access page 01002
}

func TestPDP(t *testing.T) {
	for _, tt := range pdpTests {
		rkinit()
		kb := KB11{
			buf: tt.input,
		}
		kb.Reset()
		for i := 0; i < tt.cycles; i++ {
			kb.onestep()
		}
	}
}
