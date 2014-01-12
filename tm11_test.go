package pdp11

import "testing"

var BOOTTU10 = []uint16{
	0012700,
	0172526,
	0010040,
	0012740,
	0060003,
	0000777,
}

func TestTM11Bootstrap(t *testing.T) {
	pdp := New()
	pdp.LoadBootrom(01000, BOOTTU10) // word addressed
	pdp.SetPC(02000)
	pdp.Run()
}
