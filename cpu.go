package pdp11

import "fmt"

const (
	FLAGN = 8
	FLAGZ = 4
	FLAGV = 2
	FLAGC = 1
)

const pr = false // debug

var (
	clkcounter int
	waiting    = false
	interrupts []intr
)

type intr struct{ vec, pri int }

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

var bootrom = [...]uint16{
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

func xor(x, y int) int {
	a := x & y
	b := ^x & ^y
	z := ^a & ^b
	return z
}

func xor16(x, y uint16) uint16 {
	a := x & y
	b := ^x & ^y
	z := ^a & ^b
	return z
}

type cpu struct {
	R                 [8]int // registers
	PS                uint16 // processor status
	PC                uint16 // address of current instruction
	KSP, USP          uint16 // kernel and user stack pointer
	curuser, prevuser bool

	Input  chan uint8
	unibus *unibus
	mmu    mmu
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

func interrupt(vec, pri int) {
	var i int
	if vec&1 == 1 {
		panic("Thou darst calling interrupt() with an odd vector number?")
	}
	for ; i < len(interrupts); i++ {
		if interrupts[i].pri < pri {
			break
		}
	}
	for ; i < len(interrupts); i++ {
		if interrupts[i].vec >= vec {
			break
		}
	}
	// interrupts.splice(i, 0, {vec: vec, pri: pri});
	interrupts = append(interrupts[:i], append([]intr{{vec, pri}}, interrupts[i:]...)...)
}

func (k *cpu) aget(v int, l int) int {
	if (v&7) >= 6 || (v&010 != 0) {
		l = 2
	}
	if (v & 070) == 000 {
		return -(v + 1)
	}
	var addr uint16
	switch v & 060 {
	case 000:
		v &= 7
		addr = uint16(k.R[v&7])
	case 020:
		addr = uint16(k.R[v&7])
		k.R[v&7] += l
	case 040:
		k.R[v&7] -= l
		addr = uint16(k.R[v&7])
	case 060:
		addr = k.fetch16()
		addr += uint16(k.R[v&7])
	}
	addr &= 0xFFFF
	if v&010 != 0 {
		addr = k.read16(addr)
	}
	return int(addr)
}

func (k *cpu) memread(a, l int) uint16 {
	if a < 0 {
		r := uint8(-(a + 1))
		if l == 2 {
			return uint16(k.R[r&7])
		} else {
			return uint16(k.R[r&7]) & 0xFF
		}
	}
	if l == 2 {
		return k.read16(uint16(a))
	}
	return k.read8(uint16(a))
}

func (k *cpu) memwrite(a, l int, v uint16) {
	if a < 0 {
		r := uint8(-(a + 1))
		if l == 2 {
			k.R[r&7] = int(v)
		} else {
			k.R[r&7] &= 0xFF00
			k.R[r&7] |= int(v)
		}
	} else if l == 2 {
		k.write16(uint16(a), v)
	} else {
		k.write8(uint16(a), v)
	}
}

func (k *cpu) branch(o int) {
	//printstate()
	if o&0x80 == 0x80 {
		o = -(((^o) + 1) & 0xFF)
	}
	o <<= 1
	k.R[7] += o
}

func (k *cpu) step() {
	var max, maxp, msb uint16
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
	k.PC = uint16(k.R[7])
	ia := k.mmu.decode(k.PC, false, k.curuser)
	k.R[7] += 2
	instr := int(k.unibus.read16(ia))
	d := instr & 077
	s := (instr & 07700) >> 6
	l := 2 - (instr >> 15)
	o := instr & 0xFF
	if l == 2 {
		max = 0xFFFF
		maxp = 0x7FFF
		msb = 0x8000
	} else {
		max = 0xFF
		maxp = 0x7F
		msb = 0x80
	}
	switch instr & 0070000 {
	case 0010000: // MOV
		// k.printstate()
		sa := k.aget(s, l)
		val := k.memread(sa, l)
		da := k.aget(d, l)
		k.PS &= 0xFFF1
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if val == 0 {
			k.PS |= FLAGZ
		}
		if da < 0 && l == 1 {
			l = 2
			if val&msb == msb {
				val |= 0xFF00
			}
		}
		k.memwrite(da, l, val)
		return
	case 0020000: // CMP
		sa := k.aget(s, l)
		val1 := k.memread(sa, l)
		da := k.aget(d, l)
		val2 := k.memread(da, l)
		val := (val1 - val2) & max
		k.PS &= 0xFFF0
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if (val1^val2)&msb == msb && !((val2^val)&msb == msb) {
			k.PS |= FLAGV
		}
		if val1 < val2 {
			k.PS |= FLAGC
		}
		return
	case 0030000: // BIT
		sa := k.aget(s, l)
		val1 := k.memread(sa, l)
		da := k.aget(d, l)
		val2 := k.memread(da, l)
		val := val1 & val2
		k.PS &= 0xFFF1
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&msb == msb {
			k.PS |= FLAGN
		}
		return
	case 0040000: // BIC
		sa := k.aget(s, l)
		val1 := k.memread(sa, l)
		da := k.aget(d, l)
		val2 := k.memread(da, l)
		val := (max ^ val1) & val2
		k.PS &= 0xFFF1
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&msb != 0 {
			k.PS |= FLAGN
		}
		k.memwrite(da, l, val)
		return
	case 0050000: // BIS
		sa := k.aget(s, l)
		val1 := k.memread(sa, l)
		da := k.aget(d, l)
		val2 := k.memread(da, l)
		val := val1 | val2
		k.PS &= 0xFFF1
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&msb == msb {
			k.PS |= FLAGN
		}
		k.memwrite(da, l, val)
		return
	}
	switch instr & 0170000 {
	case 0060000: // ADD
		sa := k.aget(s, 2)
		val1 := k.memread(sa, 2)
		da := k.aget(d, 2)
		val2 := k.memread(da, 2)
		val := (val1 + val2) & 0xFFFF
		k.PS &= 0xFFF0
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			k.PS |= FLAGN
		}
		if !((val1^val2)&0x8000 == 0x8000) && ((val2^val)&0x8000 == 0x8000) {
			k.PS |= FLAGV
		}
		if val1+val2 >= 0xFFFF {
			k.PS |= FLAGC
		}
		k.memwrite(da, 2, val)
		return
	case 0160000: // SUB
		sa := k.aget(s, 2)
		val1 := k.memread(sa, 2)
		da := k.aget(d, 2)
		val2 := k.memread(da, 2)
		val := (val2 - val1) & 0xFFFF
		k.PS &= 0xFFF0
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			k.PS |= FLAGN
		}
		if ((val1^val2)&0x8000 == 0x8000) && !((val2^val)&0x8000 == 0x8000) {
			k.PS |= FLAGV
		}
		if val1 > val2 {
			k.PS |= FLAGC
		}
		k.memwrite(da, 2, val)
		return
	}
	switch instr & 0177000 {
	case 0004000: // JSR
		val := k.aget(d, l)
		if val < 0 {
			break
		}
		k.push(uint16(k.R[s&7]))
		k.R[s&7] = k.R[7]
		k.R[7] = val
		return
	case 0070000: // MUL
		val1 := k.R[s&7]
		if val1&0x8000 == 0x8000 {
			val1 = -((0xFFFF ^ val1) + 1)
		}
		da := k.aget(d, l)
		val2 := int(k.memread(da, 2))
		if val2&0x8000 == 0x8000 {
			val2 = -((0xFFFF ^ val2) + 1)
		}
		val := val1 * val2
		k.R[s&7] = (val & 0xFFFF0000) >> 16
		k.R[(s&7)|1] = val & 0xFFFF
		k.PS &= 0xFFF0
		if val&0x80000000 == 0x80000000 {
			k.PS |= FLAGN
		}
		if val&0xFFFFFFFF == 0 {
			k.PS |= FLAGZ
		}
		if val < (1<<15) || val >= ((1<<15)-1) {
			k.PS |= FLAGC
		}
		return
	case 0071000: // DIV
		val1 := (k.R[s&7] << 16) | k.R[(s&7)|1]
		da := k.aget(d, l)
		val2 := int(k.memread(da, 2))
		k.PS &= 0xFFF0
		if val2 == 0 {
			k.PS |= FLAGC
			return
		}
		if val1/val2 >= 0x10000 {
			k.PS |= FLAGV
			return
		}
		k.R[s&7] = (val1 / val2) & 0xFFFF
		k.R[(s&7)|1] = (val1 % val2) & 0xFFFF
		if k.R[s&7] == 0 {
			k.PS |= FLAGZ
		}
		if k.R[s&7]&0100000 == 0100000 {
			k.PS |= FLAGN
		}
		if val1 == 0 {
			k.PS |= FLAGV
		}
		return
	case 0072000: // ASH
		val1 := k.R[s&7]
		da := k.aget(d, 2)
		val2 := uint(k.memread(da, 2) & 077)
		k.PS &= 0xFFF0
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
				k.PS |= FLAGC
			}
		} else {
			val = (val1 << val2) & 0xFFFF
			shift := 1 << (16 - val2)
			if val1&shift == shift {
				k.PS |= FLAGC
			}
		}
		k.R[s&7] = val
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&0100000 == 0100000 {
			k.PS |= FLAGN
		}
		if xor(val&0100000, val1&0100000) != 0 {
			k.PS |= FLAGV
		}
		return
	case 0073000: // ASHC
		val1 := k.R[s&7]<<16 | k.R[(s&7)|1]
		da := k.aget(d, 2)
		val2 := uint(k.memread(da, 2) & 077)
		k.PS &= 0xFFF0
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
				k.PS |= FLAGC
			}
		} else {
			val = (val1 << val2) & 0xFFFFFFFF
			if val1&(1<<(32-val2)) != 0 {
				k.PS |= FLAGC
			}
		}
		k.R[s&7] = (val >> 16) & 0xFFFF
		k.R[(s&7)|1] = val & 0xFFFF
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&0x80000000 != 0 {
			k.PS |= FLAGN
		}
		if xor(val&0x80000000, val1&0x80000000) != 0 {
			k.PS |= FLAGV
		}
		return
	case 0074000: // XOR
		val1 := uint16(k.R[s&7])
		da := k.aget(d, 2)
		val2 := k.memread(da, 2)
		val := val1 ^ val2
		k.PS &= 0xFFF1
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			k.PS |= FLAGZ
		}
		k.memwrite(da, 2, val)
		return
	case 0077000: // SOB
		k.R[s&7]--
		if k.R[s&7] != 0 {
			o &= 077
			o <<= 1
			k.R[7] -= o
		}
		return
	}
	switch instr & 0077700 {
	case 0005000: // CLR
		k.PS &= 0xFFF0
		k.PS |= FLAGZ
		da := k.aget(d, l)
		k.memwrite(da, l, 0)
		return
	case 0005100: // COM
		da := k.aget(d, l)
		val := k.memread(da, l) ^ max
		k.PS &= 0xFFF0
		k.PS |= FLAGC
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if val == 0 {
			k.PS |= FLAGZ
		}
		k.memwrite(da, l, val)
		return
	case 0005200: // INC
		da := k.aget(d, l)
		val := (k.memread(da, l) + 1) & max
		k.PS &= 0xFFF1
		if val&msb == msb {
			k.PS |= FLAGN | FLAGV
		}
		if val == 0 {
			k.PS |= FLAGZ
		}
		k.memwrite(da, l, val)
		return
	case 0005300: // DEC
		da := k.aget(d, l)
		val := (k.memread(da, l) - 1) & max
		k.PS &= 0xFFF1
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if val == maxp {
			k.PS |= FLAGV
		}
		if val == 0 {
			k.PS |= FLAGZ
		}
		k.memwrite(da, l, val)
		return
	case 0005400: // NEG
		da := k.aget(d, l)
		val := (-k.memread(da, l)) & max
		k.PS &= 0xFFF0
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if val == 0 {
			k.PS |= FLAGZ
		} else {
			k.PS |= FLAGC
		}
		if val == 0x8000 {
			k.PS |= FLAGV
		}
		k.memwrite(da, l, val)
		return
	case 0005500: // ADC
		da := k.aget(d, l)
		val := k.memread(da, l)
		if k.PS&FLAGC == FLAGC {
			k.PS &= 0xFFF0
			if (val+1)&msb == msb {
				k.PS |= FLAGN
			}
			if val == max {
				k.PS |= FLAGZ
			}
			if val == 0077777 {
				k.PS |= FLAGV
			}
			if val == 0177777 {
				k.PS |= FLAGC
			}
			k.memwrite(da, l, (val+1)&max)
		} else {
			k.PS &= 0xFFF0
			if val&msb == msb {
				k.PS |= FLAGN
			}
			if val == 0 {
				k.PS |= FLAGZ
			}
		}
		return
	case 0005600: // SBC
		da := k.aget(d, l)
		val := k.memread(da, l)
		if k.PS&FLAGC == FLAGC {
			k.PS &= 0xFFF0
			if (val-1)&msb == msb {
				k.PS |= FLAGN
			}
			if val == 1 {
				k.PS |= FLAGZ
			}
			if val != 0 {
				k.PS |= FLAGC
			}
			if val == 0100000 {
				k.PS |= FLAGV
			}
			k.memwrite(da, l, (val-1)&max)
		} else {
			k.PS &= 0xFFF0
			if val&msb == msb {
				k.PS |= FLAGN
			}
			if val == 0 {
				k.PS |= FLAGZ
			}
			if val == 0100000 {
				k.PS |= FLAGV
			}
			k.PS |= FLAGC
		}
		return
	case 0005700: // TST
		da := k.aget(d, l)
		val := k.memread(da, l)
		k.PS &= 0xFFF0
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if val == 0 {
			k.PS |= FLAGZ
		}
		return
	case 0006000: // ROR
		da := k.aget(d, l)
		val := k.memread(da, l)
		if k.PS&FLAGC == FLAGC {
			val |= max + 1
		}
		k.PS &= 0xFFF0
		if val&1 == 1 {
			k.PS |= FLAGC
		}
		if val&(max+1) != 0 {
			k.PS |= FLAGN
		}
		if !(val&max != 0) {
			k.PS |= FLAGZ
		}
		if xor16(val&1, val&(max+1)) != 0 {
			k.PS |= FLAGV
		}
		val >>= 1
		k.memwrite(da, l, val)
		return
	case 0006100: // ROL
		da := k.aget(d, l)
		val := k.memread(da, l) << 1
		if k.PS&FLAGC == FLAGC {
			val |= 1
		}
		k.PS &= 0xFFF0
		if val&(max+1) != 0 {
			k.PS |= FLAGC
		}
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if !(val&max != 0) {
			k.PS |= FLAGZ
		}
		if (val^(val>>1))&msb != 0 {
			k.PS |= FLAGV
		}
		val &= max
		k.memwrite(da, l, val)
		return
	case 0006200: // ASR
		da := k.aget(d, l)
		val := k.memread(da, l)
		k.PS &= 0xFFF0
		if val&1 == 1 {
			k.PS |= FLAGC
		}
		if val&msb == msb {
			k.PS |= FLAGN
		}
		if xor16(val&msb, val&1) != 0 {
			k.PS |= FLAGV
		}
		val = (val & msb) | (val >> 1)
		if val == 0 {
			k.PS |= FLAGZ
		}
		k.memwrite(da, l, val)
		return
	case 0006300: // ASL
		da := k.aget(d, l)
		val := k.memread(da, l)
		k.PS &= 0xFFF0
		if val&msb == msb {
			k.PS |= FLAGC
		}
		if val&(msb>>1) != 0 {
			k.PS |= FLAGN
		}
		if (val^(val<<1))&msb != 0 {
			k.PS |= FLAGV
		}
		val = (val << 1) & max
		if val == 0 {
			k.PS |= FLAGZ
		}
		k.memwrite(da, l, val)
		return
	case 0006700: // SXT
		da := k.aget(d, l)
		if k.PS&FLAGN == FLAGN {
			k.memwrite(da, l, max)
		} else {
			k.PS |= FLAGZ
			k.memwrite(da, l, 0)
		}
		return
	}
	switch instr & 0177700 {
	case 0000100: // JMP
		val := k.aget(d, 2)
		if val < 0 {
			panic("whoa!")
			break
		}
		k.R[7] = val
		return
	case 0000300: // SWAB
		da := k.aget(d, l)
		val := k.memread(da, l)
		val = ((val >> 8) | (val << 8)) & 0xFFFF
		k.PS &= 0xFFF0
		if (val & 0xFF) == 0 {
			k.PS |= FLAGZ
		}
		if val&0x80 == 0x80 {
			k.PS |= FLAGN
		}
		k.memwrite(da, l, val)
		return
	case 0006400: // MARK
		k.R[6] = k.R[7] + (instr&077)<<1
		k.R[7] = k.R[5]
		k.R[5] = int(k.pop())
		break
	case 0006500: // MFPI
		var val uint16
		da := k.aget(d, 2)
		switch {
		case da == -7:
			// val = (curuser == k.prevuser) ? R[6] : (prevuser ? k.USP : KSP);
			if k.curuser == k.prevuser {
				val = uint16(k.R[6])
			} else {
				if k.prevuser {
					val = k.USP
				} else {
					val = k.KSP
				}
			}
		case da < 0:
			panic("invalid MFPI instruction")
		default:
			val = k.unibus.read16(k.mmu.decode(uint16(da), false, k.prevuser))
		}
		k.push(val)
		k.PS &= 0xFFF0
		k.PS |= FLAGC
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			k.PS |= FLAGN
		}
		return
	case 0006600: // MTPI
		da := k.aget(d, 2)
		val := uint16(k.pop())
		switch {
		case da == -7:
			if k.curuser == k.prevuser {
				k.R[6] = int(val)
			} else {
				if k.prevuser {
					k.USP = val
				} else {
					k.KSP = val
				}
			}
		case da < 0:
			panic("invalid MTPI instrution")
		default:
			sa := k.mmu.decode(uint16(da), true, k.prevuser)
			k.unibus.write16(sa, val)
		}
		k.PS &= 0xFFF0
		k.PS |= FLAGC
		if val == 0 {
			k.PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			k.PS |= FLAGN
		}
		return
	}
	if (instr & 0177770) == 0000200 { // RTS
		k.R[7] = k.R[d&7]
		k.R[d&7] = int(k.pop())
		return
	}
	switch instr & 0177400 {
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
		if !(xor16(k.PS&FLAGN, k.PS&FLAGV) != 0) {
			k.branch(o)
		}
		return
	case 0002400:
		if xor16(k.PS&FLAGN, k.PS&FLAGV) != 0 {
			k.branch(o)
		}
		return
	case 0003000:
		if !(xor16(k.PS&FLAGN, k.PS&FLAGV) != 0) && !(k.PS&FLAGZ == FLAGZ) {
			k.branch(o)
		}
		return
	case 0003400:
		if xor16(k.PS&FLAGN, k.PS&FLAGV) != 0 || (k.PS&FLAGZ == FLAGZ) {
			k.branch(o)
		}
		return
	case 0100000:
		if k.PS&FLAGN == 0 {
			k.branch(o)
		}
		return
	case 0100400:
		if k.PS&FLAGN == FLAGN {
			k.branch(o)
		}
		return
	case 0101000:
		if !(k.PS&FLAGC == FLAGC) && !(k.PS&FLAGZ == FLAGZ) {
			k.branch(o)
		}
		return
	case 0101400:
		if (k.PS&FLAGC == FLAGC) || (k.PS&FLAGZ == FLAGZ) {
			k.branch(o)
		}
		return
	case 0102000:
		if !(k.PS&FLAGV == FLAGV) {
			k.branch(o)
		}
		return
	case 0102400:
		if k.PS&FLAGV == FLAGV {
			k.branch(o)
		}
		return
	case 0103000:
		if !(k.PS&FLAGC == FLAGC) {
			k.branch(o)
		}
		return
	case 0103400:
		if k.PS&FLAGC == FLAGC {
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
			vec = 020
		}
		prev := k.PS
		k.switchmode(false)
		k.push(prev)
		k.push(uint16(k.R[7]))
		k.R[7] = int(k.unibus.Memory[vec>>1])
		k.PS = k.unibus.Memory[(vec>>1)+1]
		if k.prevuser {
			k.PS |= (1 << 13) | (1 << 12)
		}
		return
	}
	if (instr & 0177740) == 0240 { // CL?, SE?
		if instr&020 == 020 {
			k.PS |= uint16(instr) & 017
		} else {
			k.PS &= ^(uint16(instr) & 017)
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
			val |= k.PS & 0177730
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

func (k *cpu) Reset() {
	for i := 0; i < 7; i++ {
		k.R[i] = 0
	}
	k.PS = 0
	k.PC = 0
	k.KSP = 0
	k.USP = 0
	k.Input = make(chan uint8) // unix\n
	k.curuser = false
	k.prevuser = false
	k.mmu.SR0 = 0
	k.unibus.LKS = 1 << 7
	for i := 0; i < len(k.unibus.Memory); i++ {
		k.unibus.Memory[i] = 0
	}
	for i := 0; i < len(bootrom); i++ {
		k.unibus.Memory[01000+i] = bootrom[i]
	}
	for i := 0; i < 16; i++ {
		k.mmu.pages[i] = createpage(0, 0)
	}
	k.R[7] = 02002
	k.unibus.cons.clearterminal()
	k.unibus.cons.Input = k.Input
	k.unibus.rk.rkreset()
	clkcounter = 0
	waiting = false
}
