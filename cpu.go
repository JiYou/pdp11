package pdp11

import "fmt"

const pr = false // debug

var (
	clkcounter int
	waiting    = false
)

// traps
const (
	INTBUS    = 0004
	INTINVAL  = 0010
	INTDEBUG  = 0014
	INTIOT    = 0020
	INTTTYIN  = 0060
	INTTTYOUT = 0064
	INTFAULT  = 0250
	INTCLOCK  = 0100
	INTRK     = 0220
)

func xor(a, b bool) bool { return a != b }

const (
	FLAGN = 8
	FLAGZ = 4
	FLAGV = 2
	FLAGC = 1
)

type PSW uint16

func (p PSW) N() bool { return p&FLAGN == FLAGN }
func (p PSW) Z() bool { return p&FLAGZ == FLAGZ }
func (p PSW) V() bool { return p&FLAGV == FLAGV }
func (p PSW) C() bool { return p&FLAGC == FLAGC }

func (p *PSW) testAndSetZero(v int) {
	if v == 0 {
		*p |= FLAGZ
	}
}

func (p *PSW) testAndSetNeg(v int) {
	if v != 0 {
		*p |= FLAGN
	}
}

type cpu struct {
	R                 [8]int // registers
	PS                PSW    // processor status
	pc                uint16 // address of currently executing instructoin
	KSP, USP          uint16 // kernel and user stack pointer
	curuser, prevuser bool

	Input  chan uint8
	unibus *unibus
	mmu    KT11

	interrupts [8]intr
}

func (k *cpu) switchmode(newm bool) {
	k.prevuser = k.curuser
	k.curuser = newm
	if k.prevuser {
		k.USP = uint16(k.R[6])
	} else {
		k.KSP = uint16(k.R[6])
	}
	if k.curuser {
		k.R[6] = int(k.USP)
	} else {
		k.R[6] = int(k.KSP)
	}
	k.PS &= 0007777
	if k.curuser {
		k.PS |= (1 << 15) | (1 << 14)
	}
	if k.prevuser {
		k.PS |= (1 << 13) | (1 << 12)
	}
}

func (k *cpu) read8(a uint16) uint16 {
	addr := k.mmu.decode(a, false, k.curuser)
	return k.unibus.read8(addr)
}

func (k *cpu) read16(a uint16) uint16 {
	addr := k.mmu.decode(a, false, k.curuser)
	return k.unibus.read16(addr)
}

func (k *cpu) write8(a, v uint16) {
	addr := k.mmu.decode(a, true, k.curuser)
	k.unibus.write8(addr, v)
}

func (k *cpu) write16(a, v uint16) {
	addr := k.mmu.decode(a, true, k.curuser)
	k.unibus.write16(addr, v)
}

func (k *cpu) fetch16() uint16 {
	val := k.read16(uint16(k.R[7]))
	k.R[7] += 2
	return val
}

func (k *cpu) push(v uint16) {
	k.R[6] -= 2
	k.write16(uint16(k.R[6]), v)
}

func (k *cpu) pop() uint16 {
	val := k.read16(uint16(k.R[6]))
	k.R[6] += 2
	return val
}

type trap struct {
	num int
	msg string
}

func (t trap) String() string {
	return fmt.Sprintf("trap %06o occured: %s", t.num, t.msg)
}

type regaddr uint16

func (r regaddr) register() bool { return r&0177770 == 0170000 }
func (r regaddr) address() bool  { return !r.register() }

func (k *cpu) aget(v, l uint8) regaddr {
	if (v & 070) == 000 {
		return 0170000 | regaddr(v&7)
	}
	if (v&7) >= 6 || (v&010 != 0) {
		l = 2
	}
	var addr uint16
	switch v & 060 {
	case 000:
		v &= 7
		addr = uint16(k.R[v&7])
	case 020:
		addr = uint16(k.R[v&7])
		k.R[v&7] += int(l)
	case 040:
		k.R[v&7] -= int(l)
		addr = uint16(k.R[v&7])
	case 060:
		addr = k.fetch16()
		addr += uint16(k.R[v&7])
	}
	addr &= 0xFFFF
	if v&010 != 0 {
		addr = k.read16(addr)
	}
	return regaddr(addr)
}

func (k *cpu) memread(a regaddr, l uint8) int {
	if a.register() {
		//fmt.Printf("memread: regaddr %06o\n", a)
		r := uint8(a & 7)
		if l == WORD {
			return k.R[r&7]
		} else {
			return k.R[r&7] & 0xFF
		}
	}
	if l == WORD {
		return int(k.read16(uint16(a)))
	}
	return int(k.read8(uint16(a)))
}

func (k *cpu) memwrite(a regaddr, l uint8, v int) {
	if a.register() {
		//fmt.Printf("memwrite: regaddr %06o\n", a)
		r := uint8(a & 7)
		if l == WORD {
			k.R[r&7] = v
		} else {
			k.R[r&7] &= 0xFF00
			k.R[r&7] |= v
		}
	} else if l == WORD {
		k.write16(uint16(a), uint16(v))
	} else {
		k.write8(uint16(a), uint16(v))
	}
}

func (k *cpu) branch(o int) {
	if o&0x80 == 0x80 {
		o = -(((^o) + 1) & 0xFF)
	}
	o <<= 1
	k.R[7] += o
}

type INST int

func (i INST) O() int   { return int(i) & 0xff }
func (i INST) S() uint8 { return uint8((i & 07700) >> 6) }
func (i INST) D() uint8 { return uint8(i & 077) }
func (i INST) L() uint8 { return uint8(2 - (i >> 15)) }

const (
	WORD = 2
	BYTE = 1
)

func (k *cpu) step() {
	if waiting {
		select {
		case v, ok := <-k.unibus.cons.Input:
			if ok {
				k.unibus.cons.addchar(int(v))
			}
		default:
		}
		return
	}
	k.pc = uint16(k.R[7])
	ia := k.mmu.decode(k.pc, false, k.curuser)
	k.R[7] += 2
	instr := INST(k.unibus.read16(ia))
	if pr {
		k.printstate()
	}
	switch instr & 0070000 {
	case 0010000: // MOV
		MOV(k, instr)
		return
	case 0020000: // CMP
		CMP(k, instr)
		return
	case 0030000: // BIT
		BIT(k, instr)
		return
	case 0040000: // BIC
		BIC(k, instr)
		return
	case 0050000: // BIS
		BIS(k, instr)
		return
	}
	switch instr & 0170000 {
	case 0060000: // ADD
		ADD(k, instr)
		return
	case 0160000: // SUB
		SUB(k, instr)
		return
	}
	switch instr & 0177000 {
	case 0004000: // JSR
		JSR(k, instr)
		return
	case 0070000: // MUL
		MUL(k, instr)
		return
	case 0071000: // DIV
		DIV(k, instr)
		return
	case 0072000: // ASH
		ASH(k, instr)
		return
	case 0073000: // ASHC
		ASHC(k, instr)
		return
	case 0074000: // XOR
		XOR(k, instr)
		return
	case 0077000: // SOB
		SOB(k, instr)
		return
	}
	switch instr & 0077700 {
	case 0005000: // CLR
		CLR(k, instr)
		return
	case 0005100: // COM
		COM(k, instr)
		return
	case 0005200: // INC
		INC(k, instr)
		return
	case 0005300: // DEC
		DEC(k, instr)
		return
	case 0005400: // NEG
		NEG(k, instr)
		return
	case 0005500: // ADC
		ADC(k, instr)
		return
	case 0005600: // SBC
		SBC(k, instr)
		return
	case 0005700: // TST
		TST(k, instr)
		return
	case 0006000: // ROR
		ROR(k, instr)
		return
	case 0006100: // ROL
		ROL(k, instr)
		return
	case 0006200: // ASR
		ASR(k, instr)
		return
	case 0006300: // ASL
		ASL(k, instr)
		return
	case 0006700: // SXT
		SXT(k, instr)
		return
	}
	switch instr & 0177700 {
	case 0000100: // JMP
		JMP(k, instr)
		return
	case 0000300: // SWAB
		SWAB(k, instr)
		return
	case 0006400: // MARK
		MARK(k, instr)
		break
	case 0006500: // MFPI
		MFPI(k, instr)
		return
	case 0006600: // MTPI
		MTPI(k, instr)
		return
	}
	if (instr & 0177770) == 0000200 { // RTS
		d := instr.D()
		k.R[7] = k.R[d&7]
		k.R[d&7] = int(k.pop())
		return
	}
	switch o := instr.O(); instr & 0177400 {
	case 0000400:
		k.branch(o)
		return
	case 0001000:
		if !(k.PS&FLAGZ == FLAGZ) {
			k.branch(o)
		}
		return
	case 0001400:
		if k.PS&FLAGZ == FLAGZ {
			k.branch(o)
		}
		return
	case 0002000:
		if !xor(k.PS.N(), k.PS.V()) {
			k.branch(o)
		}
		return
	case 0002400:
		if xor(k.PS.N(), k.PS.V()) {
			k.branch(o)
		}
		return
	case 0003000:
		if !xor(k.PS.N(), k.PS.V()) && !k.PS.Z() {
			k.branch(o)
		}
		return
	case 0003400:
		if xor(k.PS.N(), k.PS.V()) || k.PS.Z() {
			k.branch(o)
		}
		return
	case 0100000:
		if !k.PS.N() {
			k.branch(o)
		}
		return
	case 0100400:
		if k.PS.N() {
			k.branch(o)
		}
		return
	case 0101000:
		if !k.PS.C() && !k.PS.Z() {
			k.branch(o)
		}
		return
	case 0101400:
		if k.PS.C() || k.PS.Z() {
			k.branch(o)
		}
		return
	case 0102000:
		if !k.PS.V() {
			k.branch(o)
		}
		return
	case 0102400:
		if k.PS.V() {
			k.branch(o)
		}
		return
	case 0103000:
		if !k.PS.C() {
			k.branch(o)
		}
		return
	case 0103400:
		if k.PS.C() {
			k.branch(o)
		}
		return
	}
	if (instr&0177000) == 0104000 || instr == 3 || instr == 4 { // EMT TRAP IOT BPT
		var vec int
		switch {
		case (instr & 0177400) == 0104000:
			vec = 030
		case (instr & 0177400) == 0104400:
			vec = 034
		case instr == 3:
			vec = 014
		default:
			println("IOT")
			vec = 020
		}
		prev := uint16(k.PS)
		k.switchmode(false)
		k.push(prev)
		k.push(uint16(k.R[7]))
		k.R[7] = int(k.unibus.read16(uint18(vec)))
		k.PS = PSW(k.unibus.read16(uint18(vec + 2)))
		if k.prevuser {
			k.PS |= (1 << 13) | (1 << 12)
		}
		return
	}
	if (instr & 0177740) == 0240 { // CL?, SE?
		if instr&020 == 020 {
			k.PS |= PSW(instr) & 017
		} else {
			k.PS &= ^(PSW(instr) & 017)
		}
		return
	}
	switch instr {
	case 0000000: // HALT
		if k.curuser {
			break
		}
		fmt.Println("HALT")
		panic("HALT")
		return
	case 0000001: // WAIT
		if k.curuser {
			break
		}
		//println("WAIT")
		waiting = true
		return
	case 0000002: // RTI
		fallthrough
	case 0000006: // RTT
		k.R[7] = int(k.pop())
		val := k.pop()
		if k.curuser {
			val &= 047
			val |= uint16(k.PS) & 0177730
		}
		k.unibus.write16(0777776, val)
		return
	case 0000005: // RESET
		if k.curuser {
			return
		}
		k.unibus.cons.clearterminal()
		k.unibus.rk.rkreset()
		return
	case 0170011: // SETD ; not needed by UNIX, but used; therefore ignored
		return
	}
	panic(trap{INTINVAL, "invalid instruction"})
}

func (c *cpu) interrupt(vec, pri int) {
	if vec&1 == 1 {
		panic("Thou darst calling interrupt() with an odd vector number?")
	}
	// fast path
	if c.interrupts[0].vec == 0 {
		c.interrupts[0] = intr{vec, pri}
		return
	}
	var i int
	for ; i < len(c.interrupts); i++ {
		if c.interrupts[i].vec == 0 || c.interrupts[i].pri < pri {
			break
		}
	}
	for ; i < len(c.interrupts); i++ {
		if c.interrupts[i].vec == 0 || c.interrupts[i].vec >= vec {
			break
		}
	}
	if i == len(c.interrupts) {
		panic("interrupt table full")
	}
	for j := i + 1; j < len(c.interrupts); j++ {
		c.interrupts[j] = c.interrupts[j-1]
	}
	c.interrupts[i] = intr{vec, pri}
}

func (c *cpu) SetPC(pc uint16) { c.R[7] = int(pc) }

func (k *cpu) Reset() {
	for i := 0; i < 7; i++ {
		k.R[i] = 0
	}
	k.PS = 0
	k.KSP = 0
	k.USP = 0
	k.Input = make(chan uint8) // unix\n
	k.curuser = false
	k.prevuser = false
	k.mmu.SR0 = 0
	k.unibus.LKS = 1 << 7
	k.unibus.Reset()
	for i := 0; i < 16; i++ {
		k.mmu.pages[i].par = 0
		k.mmu.pages[i].pdr = 0
	}
	k.unibus.cons.clearterminal()
	k.unibus.cons.Input = k.Input
	k.unibus.rk.rkreset()
	clkcounter = 0
	waiting = false
}

func MOV(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	if l == WORD {
		msb = 0x8000
	}

	s := i.S()
	sa := c.aget(s, l)
	val := c.memread(sa, l)
	d := i.D()
	da := c.aget(d, l)
	c.PS &= 0xFFF1
	c.PS.testAndSetNeg(val & msb)
	c.PS.testAndSetZero(val)
	if da.register() && l == BYTE {
		l = WORD
		if val&msb == msb {
			val |= 0xFF00
		}
	}
	c.memwrite(da, l, val)
}

func CMP(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}
	s := i.S()
	sa := c.aget(s, l)
	val1 := c.memread(sa, l)
	d := i.D()
	da := c.aget(d, l)
	val2 := c.memread(da, l)
	val := (val1 - val2) & max
	c.PS &= 0xFFF0
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & msb)
	if (val1^val2)&msb == msb && !((val2^val)&msb == msb) {
		c.PS |= FLAGV
	}
	if val1 < val2 {
		c.PS |= FLAGC
	}
}

func BIT(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	if l == WORD {
		msb = 0x8000
	}
	s := i.S()
	sa := c.aget(s, l)
	val1 := c.memread(sa, l)
	d := i.D()
	da := c.aget(d, l)
	val2 := c.memread(da, l)
	val := val1 & val2
	c.PS &= 0xFFF1
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & msb)
}

func BIC(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}

	s := i.S()
	sa := c.aget(s, l)
	val1 := c.memread(sa, l)
	d := i.D()
	da := c.aget(d, l)
	val2 := c.memread(da, l)
	val := (max ^ val1) & val2
	c.PS &= 0xFFF1
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & msb)
	c.memwrite(da, l, val)
}

func BIS(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	if l == WORD {
		msb = 0x8000
	}
	s := i.S()
	sa := c.aget(s, l)
	val1 := c.memread(sa, l)
	d := i.D()
	da := c.aget(d, l)
	val2 := c.memread(da, l)
	val := val1 | val2
	c.PS &= 0xFFF1
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & msb)
	c.memwrite(da, l, val)
}

func ADD(c *cpu, i INST) {
	s := i.S()
	sa := c.aget(s, WORD)
	val1 := c.memread(sa, WORD)
	d := i.D()
	da := c.aget(d, WORD)
	val2 := c.memread(da, WORD)
	val := (val1 + val2) & 0xFFFF
	c.PS &= 0xFFF0
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & 0x8000)
	if !((val1^val2)&0x8000 == 0x8000) && ((val2^val)&0x8000 == 0x8000) {
		c.PS |= FLAGV
	}
	if int(val1)+int(val2) >= 0xFFFF {
		c.PS |= FLAGC
	}
	c.memwrite(da, WORD, val)
}

func SUB(c *cpu, i INST) {
	s := i.S()
	sa := c.aget(s, WORD)
	val1 := c.memread(sa, WORD)
	d := i.D()
	da := c.aget(d, WORD)
	val2 := c.memread(da, WORD)
	val := (val2 - val1) & 0xFFFF
	c.PS &= 0xFFF0
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & 0x8000)
	if ((val1^val2)&0x8000 == 0x8000) && !((val2^val)&0x8000 == 0x8000) {
		c.PS |= FLAGV
	}
	if val1 > val2 {
		c.PS |= FLAGC
	}
	c.memwrite(da, WORD, val)
}

func JSR(c *cpu, i INST) {
	l := i.L()
	s := i.S()
	d := i.D()
	val := c.aget(d, l)
	if val.register() {
		panic("WHAT WHAT")
	}
	c.push(uint16(c.R[s&7]))
	c.R[s&7] = c.R[7]
	c.R[7] = int(val)
}

func MUL(c *cpu, i INST) {
	l := i.L()
	s := i.S()
	val1 := c.R[s&7]
	if val1&0x8000 == 0x8000 {
		val1 = -((0xFFFF ^ val1) + 1)
	}
	d := i.D()
	da := c.aget(d, l)
	val2 := int(c.memread(da, WORD))
	if val2&0x8000 == 0x8000 {
		val2 = -((0xFFFF ^ val2) + 1)
	}
	val := val1 * val2
	c.R[s&7] = val >> 16
	c.R[(s&7)|1] = val & 0xFFFF
	c.PS &= 0xFFF0
	c.PS.testAndSetNeg(val & 0x80000000)
	c.PS.testAndSetZero(val & 0xFFFFFFFF)
	if val < (1<<15) || val >= ((1<<15)-1) {
		c.PS |= FLAGC
	}
}

func DIV(c *cpu, i INST) {

	l := i.L()
	s := i.S()
	val1 := (c.R[s&7] << 16) | c.R[(s&7)|1]
	d := i.D()
	da := c.aget(d, l)
	val2 := int(c.memread(da, WORD))
	c.PS &= 0xFFF0
	if val2 == 0 {
		c.PS |= FLAGC
		return
	}
	if val1/val2 >= 0x10000 {
		c.PS |= FLAGV
		return
	}
	c.R[s&7] = (val1 / val2) & 0xFFFF
	c.R[(s&7)|1] = (val1 % val2) & 0xFFFF
	if c.R[s&7] == 0 {
		c.PS |= FLAGZ
	}
	if c.R[s&7]&0100000 == 0100000 {
		c.PS |= FLAGN
	}
	if val1 == 0 {
		c.PS |= FLAGV
	}
}

func ASH(c *cpu, i INST) {
	s := i.S()
	val1 := c.R[s&7]
	d := i.D()
	da := c.aget(d, WORD)
	val2 := uint8(c.memread(da, WORD) & 077)
	c.PS &= 0xFFF0
	var val int
	if val2&040 != 0 {
		val2 = (077 ^ val2) + 1
		if val1&0100000 == 0100000 {
			val = 0xFFFF ^ (0xFFFF >> val2)
			val |= val1 >> val2
		} else {
			val = val1 >> val2
		}
		shift := 1 << (val2 - 1)
		if val1&shift == shift {
			c.PS |= FLAGC
		}
	} else {
		val = (val1 << val2) & 0xFFFF
		shift := 1 << (16 - val2)
		if val1&shift == shift {
			c.PS |= FLAGC
		}
	}
	c.R[s&7] = val
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & 0100000)
	if xor(val&0100000 != 0, val1&0100000 != 0) {
		c.PS |= FLAGV
	}
}

func ASHC(c *cpu, i INST) {
	s := i.S()
	val1 := c.R[s&7]<<16 | c.R[(s&7)|1]
	d := i.D()
	da := c.aget(d, WORD)
	val2 := uint8(c.memread(da, WORD) & 077)
	c.PS &= 0xFFF0
	var val int
	if val2&040 != 0 {
		val2 = (077 ^ val2) + 1
		if val1&0x80000000 == 0x80000000 {
			val = 0xFFFFFFFF ^ (0xFFFFFFFF >> val2)
			val |= val1 >> val2
		} else {
			val = val1 >> val2
		}
		if val1&(1<<(val2-1)) != 0 {
			c.PS |= FLAGC
		}
	} else {
		val = (val1 << val2) & 0xFFFFFFFF
		if val1&(1<<(32-val2)) != 0 {
			c.PS |= FLAGC
		}
	}
	c.R[s&7] = (val >> 16) & 0xFFFF
	c.R[(s&7)|1] = val & 0xFFFF
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & 0x80000000)
	if xor(val&0x80000000 != 0, val1&0x80000000 != 0) {
		c.PS |= FLAGV
	}
}

func XOR(c *cpu, i INST) {
	s := i.S()
	val1 := c.R[s&7]
	d := i.D()
	da := c.aget(d, WORD)
	val2 := c.memread(da, WORD)
	val := val1 ^ val2
	c.PS &= 0xFFF1
	c.PS.testAndSetZero(val)
	c.PS.testAndSetNeg(val & 0x8000)
	c.memwrite(da, WORD, val)
}

func SOB(c *cpu, i INST) {
	s := i.S()
	c.R[s&7]--
	if c.R[s&7] != 0 {
		o := i.O()
		o &= 077
		o <<= 1
		c.R[7] -= o
	}
}

func CLR(c *cpu, i INST) {
	l := i.L()
	c.PS &= 0xFFF0
	c.PS |= FLAGZ
	d := i.D()
	da := c.aget(d, l)
	c.memwrite(da, l, 0)
}

func COM(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}
	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l) ^ max
	c.PS &= 0xFFF0
	c.PS |= FLAGC
	c.PS.testAndSetNeg(val & msb)
	c.PS.testAndSetZero(val)
	c.memwrite(da, l, val)
}

func INC(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}
	d := i.D()
	da := c.aget(d, l)
	val := (c.memread(da, l) + 1) & max
	c.PS &= 0xFFF1
	if val&msb == msb {
		c.PS |= FLAGN | FLAGV
	}
	c.PS.testAndSetZero(val)
	c.memwrite(da, l, val)
}

func DEC(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	maxp := 0x7f
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
		maxp = 0x7FFF
	}
	d := i.D()
	da := c.aget(d, l)
	val := (c.memread(da, l) - 1) & max
	c.PS &= 0xFFF1
	c.PS.testAndSetNeg(val & msb)
	if val == maxp {
		c.PS |= FLAGV
	}
	c.PS.testAndSetZero(val)
	c.memwrite(da, l, val)
}

func NEG(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}
	d := i.D()
	da := c.aget(d, l)
	val := (-c.memread(da, l)) & max
	c.PS &= 0xFFF0
	c.PS.testAndSetNeg(val & msb)
	if val == 0 {
		c.PS |= FLAGZ
	} else {
		c.PS |= FLAGC
	}
	if val == 0x8000 {
		c.PS |= FLAGV
	}
	c.memwrite(da, l, val)
}

func ADC(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}

	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l)
	if c.PS.C() {
		c.PS &= 0xFFF0
		if (val+1)&msb == msb {
			c.PS |= FLAGN
		}
		if val == max {
			c.PS |= FLAGZ
		}
		if val == 0077777 {
			c.PS |= FLAGV
		}
		if val == 0177777 {
			c.PS |= FLAGC
		}
		c.memwrite(da, l, (val+1)&max)
	} else {
		c.PS &= 0xFFF0
		if val&msb == msb {
			c.PS |= FLAGN
		}
		if val == 0 {
			c.PS |= FLAGZ
		}
	}
}

func SBC(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}
	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l)
	if c.PS.C() {
		c.PS &= 0xFFF0
		if (val-1)&msb == msb {
			c.PS |= FLAGN
		}
		if val == 1 {
			c.PS |= FLAGZ
		}
		if val != 0 {
			c.PS |= FLAGC
		}
		if val == 0100000 {
			c.PS |= FLAGV
		}
		c.memwrite(da, l, (val-1)&max)
	} else {
		c.PS &= 0xFFF0
		if val&msb == msb {
			c.PS |= FLAGN
		}
		if val == 0 {
			c.PS |= FLAGZ
		}
		if val == 0100000 {
			c.PS |= FLAGV
		}
		c.PS |= FLAGC
	}
}

func ASR(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	if l == WORD {
		msb = 0x8000
	}
	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l)
	c.PS &= 0xFFF0
	if val&1 == 1 {
		c.PS |= FLAGC
	}
	c.PS.testAndSetNeg(val & msb)
	if xor(val&msb != 0, val&1 == 1) {
		c.PS |= FLAGV
	}
	val = (val & msb) | (val >> 1)
	c.PS.testAndSetZero(val)
	c.memwrite(da, l, val)
}

func ASL(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		msb = 0x8000
		max = 0xFFFF
	}
	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l)
	c.PS &= 0xFFF0
	if val&msb == msb {
		c.PS |= FLAGC
	}
	if val&(msb>>1) != 0 {
		c.PS |= FLAGN
	}
	if (val^(val<<1))&msb != 0 {
		c.PS |= FLAGV
	}
	val = (val << 1) & max
	c.PS.testAndSetZero(val)
	c.memwrite(da, l, val)
}

func SXT(c *cpu, i INST) {
	l := i.L()
	max := 0xFF
	if l == WORD {
		max = 0xFFFF
	}
	d := i.D()
	da := c.aget(d, l)
	if c.PS&FLAGN == FLAGN {
		c.memwrite(da, l, max)
	} else {
		c.PS |= FLAGZ
		c.memwrite(da, l, 0)
	}
}

func TST(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	if l == WORD {
		msb = 0x8000
	}

	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l)
	c.PS &= 0xFFF0
	c.PS.testAndSetNeg(val & msb)
	c.PS.testAndSetZero(val)
}

func ROR(c *cpu, i INST) {
	l := i.L()
	max := 0xFF
	if l == WORD {
		max = 0xFFFF
	}
	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l)
	if c.PS.C() {
		val |= max + 1
	}
	c.PS &= 0xFFF0
	if val&1 == 1 {
		c.PS |= FLAGC
	}
	if val&(max+1) != 0 {
		c.PS |= FLAGN
	}
	if !(val&max != 0) {
		c.PS |= FLAGZ
	}
	if xor(val&1 == 1, val&(max+1) != 0) {
		c.PS |= FLAGV
	}
	val >>= 1
	c.memwrite(da, l, val)
}

func ROL(c *cpu, i INST) {
	l := i.L()
	msb := 0x80
	max := 0xFF
	if l == WORD {
		max = 0xFFFF
		msb = 0x8000
	}
	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l) << 1
	if c.PS.C() {
		val |= 1
	}
	c.PS &= 0xFFF0
	if val&(max+1) != 0 {
		c.PS |= FLAGC
	}
	c.PS.testAndSetNeg(val & msb)
	if !(val&max != 0) {
		c.PS |= FLAGZ
	}
	if (val^(val>>1))&msb != 0 {
		c.PS |= FLAGV
	}
	val &= max
	c.memwrite(da, l, val)
}

func SWAB(c *cpu, i INST) {
	l := i.L()
	d := i.D()
	da := c.aget(d, l)
	val := c.memread(da, l)
	val = ((val >> 8) | (val << 8)) & 0xFFFF
	c.PS &= 0xFFF0
	c.PS.testAndSetZero(val & 0xff)
	c.PS.testAndSetNeg(val & 0x80)
	c.memwrite(da, l, val)
}

func JMP(c *cpu, i INST) {
	d := i.D()
	val := c.aget(d, WORD)
	if val.register() {
		panic("JMP: cannot jump to register")
	}
	c.R[7] = int(val)
}

func MARK(c *cpu, i INST) {
	c.R[6] = c.R[7] + int(i.O())<<1
	c.R[7] = c.R[5]
	c.R[5] = int(c.pop())
}

func MFPI(c *cpu, i INST) {
	var val uint16
	d := i.D()
	da := c.aget(d, WORD)
	switch {
	case da == 0170006:
		// val = (curuser == k.prevuser) ? R[6] : (prevuser ? k.USP : KSP);
		if c.curuser == c.prevuser {
			val = uint16(c.R[6])
		} else {
			if c.prevuser {
				val = c.USP
			} else {
				val = c.KSP
			}
		}
	case da.register():
		fmt.Printf("MFPI: %06o\n", da)
		panic("invalid MFPI instruction")
	default:
		val = c.unibus.read16(c.mmu.decode(uint16(da), false, c.prevuser))
	}
	c.push(val)
	c.PS &= 0xFFF0
	c.PS |= FLAGC
	c.PS.testAndSetZero(int(val))
	c.PS.testAndSetNeg(int(val & 0x8000))
}

func MTPI(c *cpu, i INST) {
	d := i.D()
	da := c.aget(d, WORD)
	val := uint16(c.pop())
	switch {
	case da == 0170006:
		if c.curuser == c.prevuser {
			c.R[6] = int(val)
		} else {
			if c.prevuser {
				c.USP = val
			} else {
				c.KSP = val
			}
		}
	case da.register():
		panic("invalid MTPI instrution")
	default:
		sa := c.mmu.decode(uint16(da), true, c.prevuser)
		c.unibus.write16(sa, val)
	}
	c.PS &= 0xFFF0
	c.PS |= FLAGC
	c.PS.testAndSetZero(int(val))
	if val&0x8000 == 0x8000 {
		c.PS |= FLAGN
	}
}
func (c *cpu) printstate() {
	var R = c.R
	fmt.Printf("R0 %06o R1 %06o R2 %06o R3 %06o R4 %06o R5 %06o R6 %06o R7 %06o\n[", R[0], R[1], R[2], R[3], R[4], R[5], R[6], R[7])
	if c.prevuser {
		fmt.Print("u")
	} else {
		fmt.Print("k")
	}
	if c.curuser {
		fmt.Print("U")
	} else {
		fmt.Print("K")
	}
	if c.PS&FLAGN != 0 {
		fmt.Print("N")
	} else {
		fmt.Print(" ")
	}
	if c.PS&FLAGZ != 0 {
		fmt.Print("Z")
	} else {
		fmt.Print(" ")
	}
	if c.PS&FLAGV != 0 {
		fmt.Print("V")
	} else {
		fmt.Print(" ")
	}
	if c.PS&FLAGC != 0 {
		fmt.Print("C")
	} else {
		fmt.Print(" ")
	}
	ia := c.mmu.decode(c.pc, false, c.curuser)
	instr := c.unibus.read16(ia)
	fmt.Printf("]  instr %06o: %06o   %s\n", c.pc, instr, c.disasm(ia))
}
