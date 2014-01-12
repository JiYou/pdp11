package pdp11

import "fmt"

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
	ia := cpu.mmu.decode(cpu.PC, false, cpu.curuser)
	instr := p.unibus.read16(ia)
	fmt.Printf("]  instr %06o: %06o   %s\n", cpu.PC, instr, p.disasm(ia))
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

	p.cpu.step()
	if len(interrupts) > 0 && interrupts[0].pri >= ((int(p.cpu.PS)>>5)&7) {
		//fmt.Printf("IRQ: %06o\n", interrupts[0].vec)
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

	p.rk.Step()
	p.cons.Step()
}

func (p *PDP1140) handleinterrupt(vec int) {
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
		p.cpu.R[7] = int(p.Memory[vec>>1])
		p.cpu.PS = p.Memory[(vec>>1)+1]
		if p.cpu.prevuser {
			p.cpu.PS |= (1 << 13) | (1 << 12)
		}
		waiting = false
	}()
	prev := p.cpu.PS
	p.cpu.switchmode(false)
	p.cpu.push(prev)
	p.cpu.push(uint16(p.cpu.R[7]))
}

func (p *PDP1140) trapat(vec int, msg string) {
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
		p.cpu.R[7] = int(p.Memory[vec>>1])
		p.cpu.PS = p.Memory[(vec>>1)+1]
		if p.cpu.prevuser {
			p.cpu.PS |= (1 << 13) | (1 << 12)
		}
		waiting = false
	}()
	if vec&1 == 1 {
		panic("Thou darst calling trapat() with an odd vector number?")
	}
	fmt.Printf("trap %06o occured: %s\n", vec, msg)
	p.printstate()

	prev = p.cpu.PS
	p.cpu.switchmode(false)
	p.cpu.push(prev)
	p.cpu.push(uint16(p.cpu.R[7]))
}

func New() *PDP1140 {
	var pdp PDP1140
	pdp.cpu.unibus = &pdp.unibus
	pdp.unibus.cpu = &pdp.cpu
	pdp.cpu.mmu.cpu = &pdp.cpu
	pdp.unibus.rk.unibus = &pdp.unibus
	pdp.unibus.rk.rkinit()
	pdp.cpu.Reset()
	return &pdp
}
