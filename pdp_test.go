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

func TestPDP(t *testing.T) {
	rkinit()
	reset()
	for {
		onestep()
	}
}
