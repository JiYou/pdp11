package pdp11

import "testing"

type regs struct {
	R0, R1, R2, R3, R4, R5, R6, R7 int
	PS                             PSW
}

type core map[uint18]uint16

type suite struct {
	name string
	regs
	core
	steps    int
	wantregs regs
}

var instrTests = []suite{
	{
		name:     "CLR R1",
		regs:     regs{R1: 0177777, R7: 001000, PS: 0000017},
		core:     core{001000: 005001},
		steps:    1,
		wantregs: regs{R7: 001002, PS: 000004},
	},
	{
		name:     "CLRB R1",
		regs:     regs{R1: 0177777, R7: 001000, PS: 0000017},
		core:     core{001000: 0105001},
		steps:    1,
		wantregs: regs{R1: 0177400, R7: 001002, PS: 000004},
	},
	{
		name:     "COM R0",
		regs:     regs{R0: 013333, R7: 001000, PS: 0000006},
		core:     core{001000: 005100},
		steps:    1,
		wantregs: regs{R0: 0164444, R7: 001002, PS: 000011},
	},
	{
		name:     "COMB R0",
		regs:     regs{R0: 013333, R7: 001000, PS: 0000006},
		core:     core{001000: 0105100},
		steps:    1,
		wantregs: regs{R0: 013044, R7: 001002, PS: 000001}, // not sure about the PS result
	},
	{
		name:     "INC R2",
		regs:     regs{R2: 000333, R7: 001000, PS: 000000},
		core:     core{001000: 005202},
		steps:    1,
		wantregs: regs{R2: 000334, R7: 001002, PS: 000000},
	},
	{
		name:     "INC R2 (set Z)",
		regs:     regs{R2: 0177777, R7: 001000, PS: 000000},
		core:     core{001000: 005202},
		steps:    1,
		wantregs: regs{R2: 000000, R7: 001002, PS: 000004},
	},
	{
		name:     "INC R2 (set N)",
		regs:     regs{R2: 0100000, R7: 001000, PS: 000000},
		core:     core{001000: 005202},
		steps:    1,
		wantregs: regs{R2: 0100001, R7: 001002, PS: 000012}, // TODO(dfc) V should not be set
	},
	{
		name:     "INCB",
		regs:     regs{R2: 000333, R7: 001000, PS: 000000},
		core:     core{001000: 0105202},
		steps:    1,
		wantregs: regs{R2: 000334, R7: 001002, PS: 000012},
	},
	{
		name:     "INCB (overflow)",
		regs:     regs{R2: 000377, R7: 001000, PS: 000000},
		core:     core{001000: 0105202},
		steps:    1,
		wantregs: regs{R2: 000000, R7: 001002, PS: 000004},
	},
	{
		name:     "DEC R5",
		regs:     regs{R5: 000001, R7: 001000, PS: 000010},
		core:     core{001000: 005305},
		steps:    1,
		wantregs: regs{R5: 000000, R7: 001002, PS: 000004},
	},
	{
		name:     "DECB R5 (underflow)",
		regs:     regs{R5: 000000, R7: 001000, PS: 000010},
		core:     core{001000: 0105305},
		steps:    1,
		wantregs: regs{R5: 000377, R7: 001002, PS: 000010},
	},
	{
		name:     "NEG R0",
		regs:     regs{R0: 000010, R7: 001000, PS: 000000},
		core:     core{001000: 005400},
		steps:    1,
		wantregs: regs{R0: 0177770, R7: 001002, PS: 000011},
	},
	{
		name:     "NEGB R0",
		regs:     regs{R0: 000010, R7: 001000, PS: 000000},
		core:     core{001000: 005400},
		steps:    1,
		wantregs: regs{R0: 0177770, R7: 001002, PS: 000011}, // not sure if this is right
	},
	{
		name:     "TST R1",
		regs:     regs{R1: 012340, R7: 001000, PS: 00003},
		core:     core{001000: 005701},
		steps:    1,
		wantregs: regs{R1: 012340, R7: 001002, PS: 000000},
	},
	{
		name:     "TSTB R1",
		regs:     regs{R1: 000377, R7: 001000, PS: 00003},
		core:     core{001000: 0105701},
		steps:    1,
		wantregs: regs{R1: 000377, R7: 001002, PS: 000010},
	},
	{
		name:     "ASR R1",
		regs:     regs{R1: 000000, R7: 001000, PS: 000011},
		core:     core{001000: 006201},
		steps:    1,
		wantregs: regs{R1: 000000, R7: 001002, PS: 000004},
	},
	{
		name:     "ASR R1 (set V)",
		regs:     regs{R1: 0177777, R7: 001000, PS: 000001},
		core:     core{001000: 006201},
		steps:    1,
		wantregs: regs{R1: 0177777, R7: 001002, PS: 000011},
	},
	{
		name:     "ASR R1",
		regs:     regs{R1: 0077777, R7: 001000, PS: 000000},
		core:     core{001000: 006201},
		steps:    1,
		wantregs: regs{R1: 0037777, R7: 001002, PS: 000003},
	},
	{
		name:     "ASL R1",
		regs:     regs{R1: 000000, R7: 001000, PS: 000011},
		core:     core{001000: 006301},
		steps:    1,
		wantregs: regs{R1: 000000, R7: 001002, PS: 000004},
	},
	{
		name:     "ASLB R1",
		regs:     regs{R1: 000377, R7: 001000, PS: 000011},
		core:     core{001000: 0106301},
		steps:    1,
		wantregs: regs{R1: 000376, R7: 001002, PS: 000011},
	},
	{
		name:     "ROR R1",
		regs:     regs{R1: 000000, R7: 001000, PS: 000001},
		core:     core{001000: 006001},
		steps:    1,
		wantregs: regs{R1: 0100000, R7: 001002, PS: 000016},
	},
	{
		name:     "RORB R1",
		regs:     regs{R1: 000301, R7: 001000, PS: 000000},
		core:     core{001000: 0106001},
		steps:    1,
		wantregs: regs{R1: 0000140, R7: 001002, PS: 000003},
	},
}

func TestInstructions(t *testing.T) {
	for _, tt := range instrTests {
		instrTest(t, tt)
	}
}

func instrTest(t *testing.T, tt suite) {
	t.Log(tt.name)
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
