package pdp11

import "testing"

type regs struct {
	R0, R1, R2, R3, R4, R5, R6, R7 int
	PS                             PSW
}

type core map[uint18]uint16

type suite struct {
	regs
	core
	steps    int
	wantregs regs
}

var comTests = []suite{
	{
		regs:     regs{R1: 0177777, R7: 001000, PS: 0000017},
		core:     core{001000: 005001}, // COM R1
		steps:    1,
		wantregs: regs{R7: 001002, PS: 000004},
	},
	{
		regs:     regs{R1: 0177777, R7: 001000, PS: 0000017},
		core:     core{001000: 0105001}, // COMB R1
		steps:    1,
		wantregs: regs{R1: 0177400, R7: 001002, PS: 000004},
	},
}

func TestCOM(t *testing.T) {
	for _, tt := range comTests {
		instrTest(t, tt)
	}
}

func instrTest(t *testing.T, tt suite) {
	cpu := New()
	cpu.LoadMemory(tt.core)
	loadRegs(&cpu.cpu, tt.regs)
	for i := 0; i < tt.steps; i++ {
		cpu.Step()
	}
	checkRegs(t, &cpu.cpu, tt.wantregs)
}

func loadRegs(c *cpu, regs regs) {
	c.R[0] = regs.R0
	c.R[1] = regs.R1
	c.R[2] = regs.R2
	c.R[3] = regs.R3
	c.R[4] = regs.R4
	c.R[5] = regs.R5
	c.R[6] = regs.R6
	c.R[7] = regs.R7
	c.PS = regs.PS
}

func checkRegs(t *testing.T, c *cpu, regs regs) {
	if c.R[0] != regs.R0 {
		t.Errorf("R0: got %06o, want %06o", c.R[0], regs.R0)
	}
	if c.R[1] != regs.R1 {
		t.Errorf("R1: got %06o, want %06o", c.R[1], regs.R1)
	}
	if c.R[2] != regs.R2 {
		t.Errorf("R2: got %06o, want %06o", c.R[2], regs.R2)
	}
	if c.R[3] != regs.R3 {
		t.Errorf("R3: got %06o, want %06o", c.R[3], regs.R3)
	}
	if c.R[4] != regs.R4 {
		t.Errorf("R4: got %06o, want %06o", c.R[4], regs.R4)
	}
	if c.R[5] != regs.R5 {
		t.Errorf("R5: got %06o, want %06o", c.R[5], regs.R5)
	}
	if c.R[6] != regs.R6 {
		t.Errorf("R6: got %06o, want %06o", c.R[6], regs.R6)
	}
	if c.R[7] != regs.R7 {
		t.Errorf("R7: got %06o, want %06o", c.R[7], regs.R7)
	}
	if c.PS != regs.PS {
		t.Errorf("PS: got %06o, want %06o", c.PS, regs.PS)
	}
}
