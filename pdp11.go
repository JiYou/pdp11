package pdp11

import "fmt"

var BOOTRK05 = map[uint18]uint16{
	002000: 0042113,         /* "KD" */
	002002: 0012706, 002004: 02000, /* MOV #boot_start, SP */
	002006: 0012700, 002010: 0000000, /* MOV #unit, R0        ; unit number */
	002012: 0010003,         /* MOV R0, R3 */
	002014: 0000303,         /* SWAB R3 */
	002016: 0006303,         /* ASL R3 */
	002020: 0006303,         /* ASL R3 */
	002022: 0006303,         /* ASL R3 */
	002024: 0006303,         /* ASL R3 */
	002026: 0006303,         /* ASL R3 */
	002030: 0012701, 002032: 0177412, /* MOV #RKDA, R1        ; csr */
	002034: 0010311,         /* MOV R3, (R1)         ; load da */
	002036: 0005041,         /* CLR -(R1)            ; clear ba */
	002040: 0012741, 002042: 0177000, /* MOV #-256.*2, -(R1)  ; load wc */
	002044: 0012741, 002046: 0000005, /* MOV #READ+GO, -(R1)  ; read & go */
	002050: 0005002,         /* CLR R2 */
	002052: 0005003,         /* CLR R3 */
	002054: 0012704, 002056: 02020, /* MOV #START+20, R4 */
	002060: 0005005, /* CLR R5 */
	002062: 0105711, /* TSTB (R1) */
	002064: 0100376, /* BPL .-2 */
	002066: 0105011, /* CLRB (R1) */
	002070: 0005007, /* CLR PC */
}

// PDP1140 represents a PDP 11/40 with 128KW of memory.
type PDP1140 struct {
	unibus
	cpu
}

func (p *PDP1140) Step() {
	defer func() {
		t := recover()
		switch t := t.(type) {
		case trap:
			p.trapat(t.num, t.msg)
		case nil:
			// ignore
		default:
			panic(t)
		}
	}()
	p.step()
}

type intr struct{ vec, pri int }

func (p *PDP1140) step() {
	if p.cpu.interrupts[0].vec > 0 && p.cpu.interrupts[0].pri >= ((int(p.cpu.PS)>>5)&7) {
		p.handleinterrupt(p.cpu.interrupts[0].vec)
		for i := 0; i < len(p.cpu.interrupts)-1; i++ {
			p.cpu.interrupts[i] = p.cpu.interrupts[i+1]
		}
		p.cpu.interrupts[len(p.cpu.interrupts)-1] = intr{0, 0}
		return
	}
	p.cpu.step()
	clkcounter++
	if clkcounter >= 40000 {
		clkcounter = 0
		p.LKS |= (1 << 7)
		if p.LKS&(1<<6) != 0 {
			p.cpu.interrupt(intCLOCK, 6)
		}
	}
	p.rk.Step()
	p.cons.Step()
}

func (p *PDP1140) handleinterrupt(vec int) {
	//fmt.Printf("IRQ: %06o\n", vec)
	defer func() {
		t := recover()
		switch t := t.(type) {
		case trap:
			p.trapat(t.num, t.msg)
		case nil:
			break
		default:
			panic(t)
		}
		p.cpu.R[7] = int(p.unibus.read16(uint18(vec)))
		p.cpu.PS = PSW(p.unibus.read16(uint18(vec + 2)))
		if p.cpu.prevuser {
			p.cpu.PS |= (1 << 13) | (1 << 12)
		}
		waiting = false
	}()
	prev := uint16(p.cpu.PS)
	p.cpu.switchmode(false)
	p.cpu.push(prev)
	p.cpu.push(uint16(p.cpu.R[7]))
}

func (p *PDP1140) trapat(vec int, msg string) {
	fmt.Printf("trap %06o occured: %s\n", vec, msg)
	p.printstate()

	var prev uint16
	defer func() {
		t := recover()
		switch t := t.(type) {
		case trap:
			fmt.Println("red stack trap!")
			p.Memory[0] = uint16(p.cpu.R[7])
			p.Memory[1] = prev
			vec = 4
			panic("fatal")
		case nil:
			break
		default:
			panic(t)
		}
		p.cpu.R[7] = int(p.unibus.read16(uint18(vec)))
		p.cpu.PS = PSW(p.unibus.read16(uint18(vec + 2)))
		if p.cpu.prevuser {
			p.cpu.PS |= (1 << 13) | (1 << 12)
		}
		waiting = false
	}()
	if vec&1 == 1 {
		panic("Thou darst calling trapat() with an odd vector number?")
	}
	prev = uint16(p.cpu.PS)
	p.cpu.switchmode(false)
	p.cpu.push(prev)
	p.cpu.push(uint16(p.cpu.R[7]))
}

func (p *PDP1140) Run() {
	for {
		p.run()
	}
}

func (p *PDP1140) run() {
	defer func() {
		t := recover()
		switch t := t.(type) {
		case trap:
			p.trapat(t.num, t.msg)
		case nil:
			// ignore
		default:
			panic(t)
		}
	}()
	for {
		p.step()
	}
}

func (p *PDP1140) Attach(unit int, name string) { p.unibus.rk.Attach(unit, name) }

// LoadMemory takes a map of addresses and their values and applies that map to
// core memory.
func (p *PDP1140) LoadMemory(code map[uint18]uint16) {
	for a, v := range code {
		p.unibus.write16(a, v)
	}
}

func New() *PDP1140 {
	var pdp PDP1140
	pdp.cpu.unibus = &pdp.unibus
	pdp.unibus.cpu = &pdp.cpu
	pdp.cpu.mmu.cpu = &pdp.cpu
	pdp.unibus.rk.unibus = &pdp.unibus
	pdp.unibus.cons.unibus = &pdp.unibus
	pdp.cpu.Reset()
	return &pdp
}
