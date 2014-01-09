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
	//	{"", N},
	//	{"STTY -LCASE\n", N},
	//	{"date\n", N}, // processor loops
	{"ls\n", N}, // read from odd address
	//{"who\n", N}, // read from no-access page 01002
	// {"cat /etc/passwd", N*N},
}

func TestPDP(t *testing.T) {
	for _, tt := range pdpTests {
		cpu := New()
		go func() {
			defer close(cpu.Input)
			for _, c := range tt.input {
				cpu.Input <- uint8(c)
			}
		}()
		for i := 0; i < tt.cycles; i++ {
			cpu.Step()
		}
	}
}
