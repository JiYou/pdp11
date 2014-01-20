package pdp11

import "fmt"

var BOOTRK05 = []uint16{
	0042113,        /* "KD" */
	0012706, 02000, /* MOV #boot_start, SP */
	0012700, 0000000, /* MOV #unit, R0        ; unit number */
	0010003,          /* MOV R0, R3 */
	0000303,          /* SWAB R3 */
	0006303,          /* ASL R3 */
	0006303,          /* ASL R3 */
	0006303,          /* ASL R3 */
	0006303,          /* ASL R3 */
	0006303,          /* ASL R3 */
	0012701, 0177412, /* MOV #RKDA, R1        ; csr */
	0010311,          /* MOV R3, (R1)         ; load da */
	0005041,          /* CLR -(R1)            ; clear ba */
	0012741, 0177000, /* MOV #-256.*2, -(R1)  ; load wc */
	0012741, 0000005, /* MOV #READ+GO, -(R1)  ; read & go */
	0005002,        /* CLR R2 */
	0005003,        /* CLR R3 */
	0012704, 02020, /* MOV #START+20, R4 */
	0005005, /* CLR R5 */
	0105711, /* TSTB (R1) */
	0100376, /* BPL .-2 */
	0105011, /* CLRB (R1) */
	0005007, /* CLR PC */
}

// PDP1140 represents a PDP 11/40 with 128KW of memory.
type PDP1140 struct {
	unibus
	cpu
}

func (p *PDP1140) printstate() {
	var cpu = p.cpu
	var R = cpu.R
	fmt.Printf("R0 %06o R1 %06o R2 %06o R3 %06o R4 %06o R5 %06o R6 %06o R7 %06o\n[", R[0], R[1], R[2], R[3], R[4], R[5], R[6], R[7])
	if cpu.prevuser {
		fmt.Print("u")
	} else {
		fmt.Print("k")
	}
	if cpu.curuser {
		fmt.Print("U")
	} else {
		fmt.Print("K")
	}
	if cpu.PS&FLAGN != 0 {
		fmt.Print("N")
	} else {
		fmt.Print(" ")
	}
	if cpu.PS&FLAGZ != 0 {
		fmt.Print("Z")
	} else {
		fmt.Print(" ")
	}
	if cpu.PS&FLAGV != 0 {
		fmt.Print("V")
	} else {
		fmt.Print(" ")
	}
	if cpu.PS&FLAGC != 0 {
		fmt.Print("C")
	} else {
		fmt.Print(" ")
	}
	ia := cpu.mmu.decode(cpu.pc, false, cpu.curuser)
	instr := p.unibus.read16(ia)
	fmt.Printf("]  instr %06o: %06o   %s\n", cpu.pc, instr, p.disasm(ia))
}

// Step steps the CPU and all perpherals once.
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
	p.cpu.step()
	if len(interrupts) > 0 && interrupts[0].pri >= ((int(p.cpu.PS)>>5)&7) {
		p.handleinterrupt(interrupts[0].vec)
		interrupts = interrupts[1:]
	}
	clkcounter++
	if clkcounter >= 40000 {
		clkcounter = 0
		p.LKS |= (1 << 7)
		if p.LKS&(1<<6) != 0 {
			interrupt(INTCLOCK, 6)
		}
	}
	if pr {
		p.printstate()
	}

	p.rk.Step()
	p.cons.Step()
}

func (p *PDP1140) handleinterrupt(vec int) {
	fmt.Printf("IRQ: %06o\n", interrupts[0].vec)
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

func (p *PDP1140) LoadBootrom(addr uint18, rom []uint16) {
	for i := 0; i < len(rom); i++ {
		p.unibus.Memory[01000+i] = rom[i]
	}
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

func New() *PDP1140 {
	var pdp PDP1140
	pdp.cpu.unibus = &pdp.unibus
	pdp.unibus.cpu = &pdp.cpu
	pdp.cpu.mmu.cpu = &pdp.cpu
	pdp.unibus.rk.unibus = &pdp.unibus
	pdp.cpu.Reset()
	return &pdp
}
