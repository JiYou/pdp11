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
	R                 = [8]uint16{0, 0, 0, 0, 0, 0, 0, 0} // registers
	KSP, USP          uint16                              // kernel and user stack pointer
	PS                uint16                              // processor status
	curPC             uint16                              // address of current instruction
	lastPCs           []uint16
	instr             uint16            // current instruction
	memory            [64 * 1024]uint16 // word addressing
	tim1, tim2        uint16
	SR0, SR2          uint16
	curuser, prevuser bool
	LKS, clkcounter   uint16
	waiting           = false
	interrupts        []intr
)

type intr struct{ vec, pri uint16 }

var pages [16]page

type page struct {
	par, pdr        uint16
	addr, len       uint32
	read, write, ed bool
}

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

func xor(x, y uint16) uint16 {
	a := x & y
	b := ^x & ^y
	z := ^a & ^b
	return z
}

func xor32(x, y uint32) uint32 {
	a := x & y
	b := ^x & ^y
	z := ^a & ^b
	return z
}

func switchmode(newm bool) {
	prevuser = curuser
	curuser = newm
	if prevuser {
		USP = R[6]
	} else {
		KSP = R[6]
	}
	if curuser {
		R[6] = USP
	} else {
		R[6] = KSP
	}
	PS &= 0007777
	if curuser {
		PS |= (1 << 15) | (1 << 14)
	}
	if prevuser {
		PS |= (1 << 13) | (1 << 12)
	}
}

func physread16(a uint32) uint16 {
	if a&1 == 1 {
		Trap(INTBUS, "read from odd address "+ostr(a, 6))
	}
	if a < 0760000 {
		return memory[a>>1]
	}
	if a == 0777546 {
		return LKS
	}
	if a == 0777570 {
		return 0173030
	}
	if a == 0777572 {
		return SR0
	}
	if a == 0777576 {
		return SR2
	}
	if a == 0777776 {
		return PS
	}
	if a&0777770 == 0777560 {
		return consread16(a)
	}
	if a&0777760 == 0777400 {
		return rkread16(a)
	}
	if a&0777600 == 0772200 || (a&0777600) == 0777600 {
		return mmuread16(a)
	}
	if a == 0776000 {
		panic("lolwut")
	}
	Trap(INTBUS, "read from invalid address "+ostr(a, 6))
	panic("unreachable")
}

func physread8(a uint32) uint16 {
	var val uint16
	const MASK uint32 = 1
	val = physread16(a & ^MASK)
	if a&1 != 0 {
		return val >> 8
	}
	return val & 0xFF
}

func physwrite8(a uint32, v uint16) {
	if a < 0760000 {
		if a&1 == 1 {
			memory[a>>1] &= 0xFF
			memory[a>>1] |= (v & 0xFF) << 8
		} else {
			memory[a>>1] &= 0xFF00
			memory[a>>1] |= v & 0xFF
		}
	} else {
		if a&1 == 1 {
			physwrite16(a&^1, (physread16(a)&0xFF)|(v&0xFF)<<8)
		} else {
			physwrite16(a&^1, (physread16(a)&0xFF00)|(v&0xFF))
		}
	}
}

func physwrite16(a uint32, v uint16) {
	if a%1 != 0 {
		Trap(INTBUS, "write to odd address "+ostr(a, 6))
	}
	if a < 0760000 {
		memory[a>>1] = v
	} else if a == 0777776 {
		switch v >> 14 {
		case 0:
			switchmode(false)
			break
		case 3:
			switchmode(true)
			break
		default:
			panic("invalid mode")
		}
		switch (v >> 12) & 3 {
		case 0:
			prevuser = false
			break
		case 3:
			prevuser = true
			break
		default:
			panic("invalid mode")
		}
		PS = v
	} else if a == 0777546 {
		LKS = v
	} else if a == 0777572 {
		SR0 = v
	} else if (a & 0777770) == 0777560 {
		conswrite16(a, v)
	} else if (a & 0777700) == 0777400 {
		rkwrite16(a, v)
	} else if (a&0777600) == 0772200 || (a&0777600) == 0777600 {
		mmuwrite16(a, v)
	} else {
		Trap(INTBUS, "write to invalid address "+ostr(a, 6))
	}
}

func decode(a uint32, w, m bool) uint32 {
	var p page
	var user uint32
	var block, disp uint32
	if !(SR0&1 == 1) {
		if a >= 0170000 {
			a += 0600000
		}
		return a
	}
	if m {
		user = 8
	} else {
		user = 0
	}
	p = pages[(a>>13)+user]
	if w && !p.write {
		SR0 = (1 << 13) | 1
		SR0 |= uint16(a>>12) & ^uint16(1)
		if user != 0 {
			SR0 |= (1 << 5) | (1 << 6)
		}
		SR2 = curPC
		Trap(INTFAULT, "write to read-only page "+ostr(a, 6))
	}
	if !p.read {
		SR0 = (1 << 15) | 1
		SR0 |= uint16(a>>12) & ^uint16(1)
		if user != 0 {
			SR0 |= (1 << 5) | (1 << 6)
		}
		SR2 = curPC
		Trap(INTFAULT, "read from no-access page "+ostr(a, 6))
	}
	block = a >> 6 & 0177
	disp = a & 077
	if p.ed && block < p.len || !p.ed && block > p.len {
		//if(p.ed ? (block < p.len) : (block > p.len)) {
		SR0 = (1 << 14) | 1
		SR0 |= uint16(a>>12) & ^uint16(1)
		if user > 0 {
			SR0 |= (1 << 5) | (1 << 6)
		}
		SR2 = curPC
		Trap(INTFAULT, "page length exceeded, address "+ostr(a, 6)+" (block "+ostr(uint32(block), 3)+") is beyond length "+ostr(uint32(p.len), 3))
	}
	if w {
		p.pdr |= 1 << 6
	}
	return ((block + p.addr) << 6) + disp
}

func createpage(par, pdr uint16) page {
	return page{
		par:   par,
		pdr:   pdr,
		addr:  uint32(par & 07777),
		len:   uint32(pdr>>8) & 0x7F,
		read:  (pdr & 2) == 2,
		write: (pdr & 6) == 6,
		ed:    (pdr & 8) == 8,
	}
}

func mmuread16(a uint32) uint16 {
	var i uint16
	i = uint16((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		return pages[i].pdr
	}
	if (a >= 0772340) && (a < 0772360) {
		return pages[i].par
	}
	if (a >= 0777600) && (a < 0777620) {
		return pages[i+8].pdr
	}
	if (a >= 0777640) && (a < 0777660) {
		return pages[i+8].par
	}
	Trap(INTBUS, "invalid read from "+ostr(a, 6))
	panic("unreachable")
}

func mmuwrite16(a uint32, v uint16) {
	var i uint16
	i = uint16((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		pages[i] = createpage(pages[i].par, v)
		return
	}
	if (a >= 0772340) && (a < 0772360) {
		pages[i] = createpage(v, pages[i].pdr)
		return
	}
	if (a >= 0777600) && (a < 0777620) {
		pages[i+8] = createpage(pages[i+8].par, v)
		return
	}
	if (a >= 0777640) && (a < 0777660) {
		pages[i+8] = createpage(v, pages[i+8].pdr)
		return
	}
	Trap(INTBUS, "write to invalid address "+ostr(a, 6))
	panic("unreachable")
}

func read8(a uint16) uint16 {
	return physread8(decode(uint32(a), false, curuser))
}

func read16(a uint16) uint16 {
	return physread16(decode(uint32(a), false, curuser))
}

func write8(a, v uint16) {
	physwrite8(decode(uint32(a), true, curuser), v)
}

func write16(a, v uint16) {
	physwrite16(decode(uint32(a), true, curuser), v)
}

func fetch16() uint16 {
	val := read16(R[7])
	R[7] += 2
	return val
}

func push(v uint16) {
	R[6] -= 2
	write16(R[6], v)
}

// func pop(v uint16) uint16 {
func pop() uint16 {
	var val uint16
	val = read16(R[6])
	R[6] += 2
	return val
}

func ostr(z interface{}, n uint16) string {
	return fmt.Sprintf("%#o", z)
}

var writedebug = fmt.Print

func printstate() {
	writedebug("R0 " + ostr(R[0], 6) + " " +
		"R1 " + ostr(R[1], 6) + " " +
		"R2 " + ostr(R[2], 6) + " " +
		"R3 " + ostr(R[3], 6) + " " +
		"R4 " + ostr(R[4], 6) + " " +
		"R5 " + ostr(R[5], 6) + " " +
		"R6 " + ostr(R[6], 6) + " " +
		"R7 " + ostr(R[7], 6) + "\n[")
	if prevuser {
		writedebug("u")
	} else {
		writedebug("k")
	}
	if curuser {
		writedebug("U")
	} else {
		writedebug("K")
	}
	if PS&FLAGN != 0 {
		writedebug("N")
	} else {
		writedebug(" ")
	}
	if PS&FLAGZ != 0 {
		writedebug("Z")
	} else {
		writedebug(" ")
	}
	if PS&FLAGV != 0 {
		writedebug("V")
	} else {
		writedebug(" ")
	}
	if PS&FLAGC != 0 {
		writedebug("C")
	} else {
		writedebug(" ")
	}
	writedebug("]  instr " + ostr(curPC, 6) + ": " + ostr(instr, 6) + "   ")
	writedebug(disasm(decode(uint32(curPC), false, curuser)))
	writedebug("\n")
}

type trap struct {
	num uint16
	msg string
}

func Trap(num uint16, msg string) {
	panic(trap{num, msg})
}

func (t trap) String() string {
	return fmt.Sprintf("trap %#d occured: %s", t.num, t.msg)
}

func interrupt(vec, pri uint16) {
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
	fmt.Println("Interrupts:", interrupts)
}

func handleinterrupt(vec uint16) {
	fmt.Println("handleinterrupt vec:", vec)
	defer func() {
		trap := recover()
		switch trap := trap.(type) {
		case struct {
			num uint16
			msg string
		}:
			trapat(trap.num, trap.msg)
		case nil:
			break
		default:
			panic(trap)
		}
		R[7] = memory[vec>>1]
		PS = memory[(vec>>1)+1]
		if prevuser {
			PS |= (1 << 13) | (1 << 12)
		}
		waiting = false
	}()
	prev := PS
	switchmode(false)
	push(prev)
	push(R[7])
}

func trapat(vec uint16, msg string) {
	var prev uint16
	defer func() {
		trap := recover()
		switch trap := trap.(type) {
		case struct {
			num uint16
			msg string
		}:
			writedebug("red stack trap!\n")
			memory[0] = R[7]
			memory[1] = prev
			vec = 4
		case nil:
			break
		default:
			panic(trap)
		}
		R[7] = memory[vec>>1]
		PS = memory[(vec>>1)+1]
		if prevuser {
			PS |= (1 << 13) | (1 << 12)
		}
		waiting = false
	}()
	if vec&1 == 1 {
		panic("Thou darst calling trapat() with an odd vector number?")
	}
	writedebug("trap " + ostr(vec, 6) + " occured: " + msg + "\n")
	printstate()

	prev = PS
	switchmode(false)
	push(prev)
	push(R[7])
}

func aget(v, l uint16) int {
	if (v&7) >= 6 || (v&010 == 010) {
		l = 2
	}
	if (v & 070) == 000 {
		return -int(v + 1)
	}
	var addr uint16
	switch v & 060 {
	case 000:
		v &= 7
		addr = R[v&7]
	case 020:
		addr = R[v&7]
		R[v&7] += l
	case 040:
		R[v&7] -= l
		addr = R[v&7]
	case 060:
		addr = fetch16()
		addr += R[v&7]
	}
	addr &= 0xFFFF
	if v&010 == 010 {
		addr = read16(addr)
	}
	return int(addr)
}

func memread(a int, l uint16) uint16 {
	if a < 0 {
		if l == 2 {
			return R[-(a + 1)]
		} else {
			return R[-(a+1)] & 0xFF
		}
	}
	if l == 2 {
		return read16(uint16(a))
	}
	return read8(uint16(a))
}

func memwrite(a int, l, v uint16) {
	if a < 0 {
		if l == 2 {
			R[-(a + 1)] = v
		} else {
			R[-(a + 1)] &= 0xFF00
			R[-(a + 1)] |= v
		}
	} else if l == 2 {
		write16(uint16(a), v)
	} else {
		write8(uint16(a), v)
	}
}

func branch(o uint16) {
	//printstate()
	if o&0x80 == 0x80 {
		o = -(((^o) + 1) & 0xFF)
	}
	o <<= 1
	R[7] += o
}

func step() {
	var max, maxp, msb uint16
	if waiting {
		return
	}
	curPC = R[7]
	ia := decode(uint32(R[7]), false, curuser)
	R[7] += 2
	//lastPCs = lastPCs.slice(0, 100)
	//lastPCs.splice(0, 0, ia)
	instr = physread16(ia)
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
		printstate()
		sa := aget(s, l)
		val := memread(sa, l)
		da := aget(d, l)
		PS &= 0xFFF1
		if val&msb == msb {
			PS |= FLAGN
		}
		if val == 0 {
			PS |= FLAGZ
		}
		if da < 0 && l == 1 {
			l = 2
			if val&msb == msb {
				val |= 0xFF00
			}
		}
		memwrite(da, l, val)
		return
	case 0020000: // CMP
		sa := aget(s, l)
		val1 := memread(sa, l)
		da := aget(d, l)
		val2 := memread(da, l)
		val := (val1 - val2) & max
		PS &= 0xFFF0
		if val == 0 {
			PS |= FLAGZ
		}
		if val&msb == msb {
			PS |= FLAGN
		}
		if (val1^val2)&msb == msb && !((val2^val)&msb == msb) {
			PS |= FLAGV
		}
		if val1 < val2 {
			PS |= FLAGC
		}
		return
	case 0030000: // BIT
		sa := aget(s, l)
		val1 := memread(sa, l)
		da := aget(d, l)
		val2 := memread(da, l)
		val := val1 & val2
		PS &= 0xFFF1
		if val == 0 {
			PS |= FLAGZ
		}
		if val&msb == msb {
			PS |= FLAGN
		}
		return
	case 0040000: // BIC
		sa := aget(s, l)
		val1 := memread(sa, l)
		da := aget(d, l)
		val2 := memread(da, l)
		val := (max ^ val1) & val2
		PS &= 0xFFF1
		if val == 0 {
			PS |= FLAGZ
		}
		if val&msb != 0 {
			PS |= FLAGN
		}
		memwrite(da, l, val)
		return
	case 0050000: // BIS
		sa := aget(s, l)
		val1 := memread(sa, l)
		da := aget(d, l)
		val2 := memread(da, l)
		val := val1 | val2
		PS &= 0xFFF1
		if val == 0 {
			PS |= FLAGZ
		}
		if val&msb != 0 {
			PS |= FLAGN
		}
		memwrite(da, l, val)
		return
	}
	switch instr & 0170000 {
	case 0060000: // ADD
		sa := aget(s, 2)
		val1 := memread(sa, 2)
		da := aget(d, 2)
		val2 := memread(da, 2)
		val := (val1 + val2) & 0xFFFF
		PS &= 0xFFF0
		if val == 0 {
			PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			PS |= FLAGN
		}
		if !((val1^val2)&0x8000 == 0x8000) && ((val2^val)&0x8000 == 0x8000) {
			PS |= FLAGV
		}
		if int(val1)+int(val2) >= 0xFFFF {
			PS |= FLAGC
		}
		memwrite(da, 2, val)
		return
	case 0160000: // SUB
		sa := aget(s, 2)
		val1 := memread(sa, 2)
		da := aget(d, 2)
		val2 := memread(da, 2)
		val := (val2 - val1) & 0xFFFF
		PS &= 0xFFF0
		if val == 0 {
			PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			PS |= FLAGN
		}
		if ((val1^val2)&0x8000 == 0x8000) && !((val2^val)&0x8000 == 0x8000) {
			PS |= FLAGV
		}
		if val1 > val2 {
			PS |= FLAGC
		}
		memwrite(da, 2, val)
		return
	}
	switch instr & 0177000 {
	case 0004000: // JSR
		val := aget(d, l)
		if val < 0 {
			break
		}
		push(R[s&7])
		R[s&7] = R[7]
		R[7] = uint16(val)
		return
	case 0070000: // MUL
		printstate()
		val1 := R[s&7]
		if val1&0x8000 == 0x8000 {
			val1 = -((0xFFFF ^ val1) + 1)
		}
		da := aget(d, l)
		val2 := memread(da, 2)
		if val2&0x8000 == 0x8000 {
			val2 = -((0xFFFF ^ val2) + 1)
		}
		val3 := uint32(val1) * uint32(val2)
		R[s&7] = uint16((val3 & 0xFFFF0000) >> 16)
		R[(s&7)|1] = uint16(val3 & 0xFFFF)
		PS &= 0xFFF0
		if val3&0x80000000 != 0 {
			PS |= FLAGN
		}
		if (val3 & 0xFFFFFFFF) == 0 {
			PS |= FLAGZ
		}
		if val3 < (1<<15) || val3 >= ((1<<15)-1) {
			PS |= FLAGC
		}
		return
	case 0071000: // DIV
		val1 := (R[s&7] << 16) | R[(s&7)|1]
		da := aget(d, l)
		val2 := memread(da, 2)
		PS &= 0xFFF0
		if val2 == 0 {
			PS |= FLAGC
			return
		}
		if int(val1/val2) >= 0x10000 {
			PS |= FLAGV
			return
		}
		R[s&7] = (val1 / val2) & 0xFFFF
		R[(s&7)|1] = (val1 % val2) & 0xFFFF
		if R[s&7] == 0 {
			PS |= FLAGZ
		}
		if R[s&7]&0100000 == 0100000 {
			PS |= FLAGN
		}
		if val1 == 0 {
			PS |= FLAGV
		}
		return
	case 0072000: // ASH
		val1 := R[s&7]
		da := aget(d, 2)
		val2 := memread(da, 2) & 077
		PS &= 0xFFF0
		var val uint16
		if val2&040 == 040 {
			val2 = (077 ^ val2) + 1
			if val1&0100000 == 0100000 {
				val = 0xFFFF ^ (0xFFFF >> val2)
				val |= val1 >> val2
			} else {
				val = val1 >> val2
			}
			if val1&(1<<(val2-1)) != 0 {
				PS |= FLAGC
			}
		} else {
			val = (val1 << val2) & 0xFFFF
			if val1&(1<<(16-val2)) != 0 {
				PS |= FLAGC
			}
		}
		R[s&7] = val
		if val == 0 {
			PS |= FLAGZ
		}
		if val&0100000 != 0 {
			PS |= FLAGN
		}
		if xor(val&0100000, val1&0100000) != 0 {
			PS |= FLAGV
		}
		return
	case 0073000: // ASHC
		var val uint32
		val1 := uint32(R[s&7])<<16 | uint32(R[(s&7)|1])
		da := aget(d, 2)
		val2 := memread(da, 2) & 077
		PS &= 0xFFF0
		if val2&040 == 040 {
			val2 = (077 ^ val2) + 1
			if val1&0x80000000 != 0 {
				val = 0xFFFFFFFF ^ (0xFFFFFFFF >> val2)
				val |= val1 >> val2
			} else {
				val = val1 >> val2
			}
			if val1&(1<<(val2-1)) != 0 {
				PS |= FLAGC
			}
		} else {
			val = (val1 << val2) & 0xFFFFFFFF
			if val1&(1<<(32-val2)) != 0 {
				PS |= FLAGC
			}
		}
		R[s&7] = uint16((val >> 16) & 0xFFFF)
		R[(s&7)|1] = uint16(val & 0xFFFF)
		if val == 0 {
			PS |= FLAGZ
		}
		if val&0x80000000 != 0 {
			PS |= FLAGN
		}
		if xor32(val&0x80000000, val1&0x80000000) != 0 {
			PS |= FLAGV
		}
		return
	case 0074000: // XOR
		val1 := R[s&7]
		da := aget(d, 2)
		val2 := memread(da, 2)
		val := val1 ^ val2
		PS &= 0xFFF1
		if val == 0 {
			PS |= FLAGZ
		}
		if val&0x8000 == 0x8000 {
			PS |= FLAGZ
		}
		memwrite(da, 2, val)
		return
	case 0077000: // SOB
		R[s&7]--
		if R[s&7] != 0 {
			o &= 077
			o <<= 1
			R[7] -= o
		}
		return
	}
	switch instr & 0077700 {
	case 0005000: // CLR
		PS &= 0xFFF0
		PS |= FLAGZ
		da := aget(d, l)
		memwrite(da, l, 0)
		return
	case 0005100: // COM
		da := aget(d, l)
		val := memread(da, l) ^ max
		PS &= 0xFFF0
		PS |= FLAGC
		if val&msb != 0 {
			PS |= FLAGN
		}
		if val == 0 {
			PS |= FLAGZ
		}
		memwrite(da, l, val)
		return
	case 0005200: // INC
		da := aget(d, l)
		val := (memread(da, l) + 1) & max
		PS &= 0xFFF1
		if val&msb != 0 {
			PS |= FLAGN | FLAGV
		}
		if val == 0 {
			PS |= FLAGZ
		}
		memwrite(da, l, val)
		return
	case 0005300: // DEC
		da := aget(d, l)
		val := (memread(da, l) - 1) & max
		PS &= 0xFFF1
		if val&msb != 0 {
			PS |= FLAGN
		}
		if val == maxp {
			PS |= FLAGV
		}
		if val == 0 {
			PS |= FLAGZ
		}
		memwrite(da, l, val)
		return
	case 0005400: // NEG
		da := aget(d, l)
		val := (-memread(da, l)) & max
		PS &= 0xFFF0
		if val&msb != 0 {
			PS |= FLAGN
		}
		if val == 0 {
			PS |= FLAGZ
		} else {
			PS |= FLAGC
		}
		if val == 0x8000 {
			PS |= FLAGV
		}
		memwrite(da, l, val)
		return
	case 0005500: // ADC
		da := aget(d, l)
		val := memread(da, l)
		if PS&FLAGC == FLAGC {
			PS &= 0xFFF0
			if (val+1)&msb != 0 {
				PS |= FLAGN
			}
			if val == max {
				PS |= FLAGZ
			}
			if val == 0077777 {
				PS |= FLAGV
			}
			if val == 0177777 {
				PS |= FLAGC
			}
			memwrite(da, l, (val+1)&max)
		} else {
			PS &= 0xFFF0
			if val&msb != 0 {
				PS |= FLAGN
			}
			if val == 0 {
				PS |= FLAGZ
			}
		}
		return
	case 0005600: // SBC
		da := aget(d, l)
		val := memread(da, l)
		if PS&FLAGC == FLAGC {
			PS &= 0xFFF0
			if (val-1)&msb != 0 {
				PS |= FLAGN
			}
			if val == 1 {
				PS |= FLAGZ
			}
			if val != 0 {
				PS |= FLAGC
			}
			if val == 0100000 {
				PS |= FLAGV
			}
			memwrite(da, l, (val-1)&max)
		} else {
			PS &= 0xFFF0
			if val&msb != 0 {
				PS |= FLAGN
			}
			if val == 0 {
				PS |= FLAGZ
			}
			if val == 0100000 {
				PS |= FLAGV
			}
			PS |= FLAGC
		}
		return
	case 0005700: // TST
		da := aget(d, l)
		val := memread(da, l)
		PS &= 0xFFF0
		if val&msb != 0 {
			PS |= FLAGN
		}
		if val == 0 {
			PS |= FLAGZ
		}
		return
	case 0006000: // ROR
		da := aget(d, l)
		val := memread(da, l)
		if PS&FLAGC == FLAGC {
			val |= max + 1
		}
		PS &= 0xFFF0
		if val&1 == 1 {
			PS |= FLAGC
		}
		if val&(max+1) != 0 {
			PS |= FLAGN
		}
		if !(val&max != 0) {
			PS |= FLAGZ
		}
		if xor(val&1, val&(max+1)) != 0 {
			PS |= FLAGV
		}
		val >>= 1
		memwrite(da, l, val)
		return
	case 0006100: // ROL
		da := aget(d, l)
		val := memread(da, l) << 1
		if PS&FLAGC == FLAGC {
			val |= 1
		}
		PS &= 0xFFF0
		if val&(max+1) != 0 {
			PS |= FLAGC
		}
		if val&msb != 0 {
			PS |= FLAGN
		}
		if !(val&max != 0) {
			PS |= FLAGZ
		}
		if (val^(val>>1))&msb != 0 {
			PS |= FLAGV
		}
		val &= max
		memwrite(da, l, val)
		return
	case 0006200: // ASR
		da := aget(d, l)
		val := memread(da, l)
		PS &= 0xFFF0
		if val&1 != 0 {
			PS |= FLAGC
		}
		if val&msb != 0 {
			PS |= FLAGN
		}
		if xor(val&msb, val&1) != 0 {
			PS |= FLAGV
		}
		val = (val & msb) | (val >> 1)
		if val == 0 {
			PS |= FLAGZ
		}
		memwrite(da, l, val)
		return
	case 0006300: // ASL
		da := aget(d, l)
		val := memread(da, l)
		PS &= 0xFFF0
		if val&msb != 0 {
			PS |= FLAGC
		}
		if val&(msb>>1) != 0 {
			PS |= FLAGN
		}
		if (val^(val<<1))&msb != 0 {
			PS |= FLAGV
		}
		val = (val << 1) & max
		if val == 0 {
			PS |= FLAGZ
		}
		memwrite(da, l, val)
		return
	case 0006700: // SXT
		da := aget(d, l)
		if PS&FLAGN == FLAGN {
			memwrite(da, l, max)
		} else {
			PS |= FLAGZ
			memwrite(da, l, 0)
		}
		return
	}
	switch instr & 0177700 {
	case 0000100: // JMP
		val := aget(d, 2)
		if val < 0 {
			break
		}
		R[7] = uint16(val)
		return
	case 0000300: // SWAB
		da := aget(d, l)
		val := memread(da, l)
		val = ((val >> 8) | (val << 8)) & 0xFFFF
		PS &= 0xFFF0
		if (val & 0xFF) == 0 {
			PS |= FLAGZ
		}
		if val&0x80 == 0x80 {
			PS |= FLAGN
		}
		memwrite(da, l, val)
		return
	case 0006400: // MARK
		R[6] = R[7] + (instr&077)<<1
		R[7] = R[5]
		R[5] = pop()
		break
	case 0006500: // MFPI
		var val uint16
		da := aget(d, 2)
		if da == -7 {
			if curuser == prevuser {
				val = R[6]
			} else if prevuser {
				val = USP
			} else {
				val = KSP
			}
			// val = (curuser == prevuser) ? R[6] : (prevuser ? USP : KSP);
		} else if da < 0 {
			panic("invalid MFPI instruction")
		} else {
			val = physread16(decode(uint32(da), false, prevuser))
		}
		push(val)
		PS &= 0xFFF0
		PS |= FLAGC
		if val == 0 {
			PS |= FLAGZ
		}
		if val&0x8000 != 0 {
			PS |= FLAGN
		}
		return
	case 0006600: // MTPI
		da := aget(d, 2)
		val := pop()
		if da == -7 {
			if curuser == prevuser {
				R[6] = val
			} else if prevuser {
				USP = val
			} else {
				KSP = val
			}
		} else if da < 0 {
			panic("invalid MTPI instrution")
		} else {
			sa := decode(uint32(da), true, prevuser)
			physwrite16(sa, val)
		}
		PS &= 0xFFF0
		PS |= FLAGC
		if val == 0 {
			PS |= FLAGZ
		}
		if val&0x8000 != 0 {
			PS |= FLAGN
		}
		return
	}
	if (instr & 0177770) == 0000200 { // RTS
		R[7] = R[d&7]
		R[d&7] = pop()
		return
	}
	switch instr & 0177400 {
	case 0000400:
		branch(o)
		return
	case 0001000:
		if !(PS&FLAGZ == FLAGZ) {
			branch(o)
		}
		return
	case 0001400:
		if PS&FLAGZ == FLAGZ {
			branch(o)
		}
		return
	case 0002000:
		if !(xor(PS&FLAGN, PS&FLAGV) != 0) {
			branch(o)
		}
		return
	case 0002400:
		if xor(PS&FLAGN, PS&FLAGV) != 0 {
			branch(o)
		}
		return
	case 0003000:
		if !(xor(PS&FLAGN, PS&FLAGV) != 0) && !(PS&FLAGZ == FLAGZ) {
			branch(o)
		}
		return
	case 0003400:
		if xor(PS&FLAGN, PS&FLAGV) != 0 || (PS&FLAGZ == FLAGZ) {
			branch(o)
		}
		return
	case 0100000:
		if PS&FLAGN == 0 {
			branch(o)
		}
		return
	case 0100400:
		if PS&FLAGN == FLAGN {
			branch(o)
		}
		return
	case 0101000:
		if !(PS&FLAGC == FLAGC) && !(PS&FLAGZ == FLAGZ) {
			branch(o)
		}
		return
	case 0101400:
		if (PS&FLAGC == FLAGC) || (PS&FLAGZ == FLAGZ) {
			branch(o)
		}
		return
	case 0102000:
		if !(PS&FLAGV == FLAGV) {
			branch(o)
		}
		return
	case 0102400:
		if PS&FLAGV == FLAGV {
			branch(o)
		}
		return
	case 0103000:
		if !(PS&FLAGC == FLAGC) {
			branch(o)
		}
		return
	case 0103400:
		if PS&FLAGC == FLAGC {
			branch(o)
		}
		return
	}
	if (instr&0177000) == 0104000 || instr == 3 || instr == 4 { // EMT TRAP IOT BPT
		var vec, prev uint16
		if (instr & 0177400) == 0104000 {
			vec = 030
		} else if (instr & 0177400) == 0104400 {
			vec = 034
		} else if instr == 3 {
			vec = 014
		} else {
			vec = 020
		}
		prev = PS
		switchmode(false)
		push(prev)
		push(R[7])
		R[7] = memory[vec>>1]
		PS = memory[(vec>>1)+1]
		if prevuser {
			PS |= (1 << 13) | (1 << 12)
		}
		return
	}
	if (instr & 0177740) == 0240 { // CL?, SE?
		if instr&020 == 020 {
			PS |= instr & 017
		} else {
			PS &= ^(instr & 017)
		}
		return
	}
	switch instr {
	case 0000000: // HALT
		if curuser {
			break
		}
		writedebug("HALT\n")
		printstate()
		panic("HALT")
		return
	case 0000001: // WAIT
		//		stop();
		//		setTimeout('LKS |= 0x80; interrupt(INTCLOCK, 6); run();', 20); // FIXME, really
		if curuser {
			break
		}
		waiting = true
		return
	case 0000002: // RTI
	case 0000006: // RTT
		R[7] = pop()
		val := pop()
		if curuser {
			val &= 047
			val |= PS & 0177730
		}
		physwrite16(0777776, val)
		return
	case 0000005: // RESET
		if curuser {
			return
		}
		clearterminal()
		rkreset()
		return
	case 0170011: // SETD ; not needed by UNIX, but used; therefore ignored
		return
	}
	fmt.Println(ia, disasm(ia))
	dumpmem()
	Trap(INTINVAL, "invalid instruction")
}

func reset() {
	for i := 0; i < 7; i++ {
		R[i] = 0
	}
	PS = 0
	KSP = 0
	USP = 0
	curuser = false
	prevuser = false
	SR0 = 0
	curPC = 0
	instr = 0
	LKS = 1 << 7
	for i := 0; i < len(memory); i++ {
		memory[i] = 0
	}
	for i := 0; i < len(bootrom); i++ {
		memory[01000+i] = bootrom[i]
	}
	for i := 0; i < 16; i++ {
		pages[i] = createpage(0, 0)
	}
	R[7] = 02002
	clearterminal()
	rkreset()
	clkcounter = 0
	waiting = false
}

func nsteps(n int) {
	for ; n != 0; n-- {
		//		try {
		step()
		if len(interrupts) > 0 && interrupts[0].pri >= ((PS>>5)&7) {
			handleinterrupt(interrupts[0].vec)
			interrupts = interrupts[1:]
		}
		clkcounter++
		if clkcounter >= 40000 {
			clkcounter = 0
			LKS |= (1 << 7)
			if LKS&(1<<6) != 0 {
				interrupt(INTCLOCK, 6)
			}
		}
		//		} catch(e) {
		//			if(e.num != undefined) {
		//				trapat(e.num, e.msg);
		//			} else throw e;
		//		}
		if pr {
			printstate()
		}
	}
}

func run() {
	//	for {
	nsteps(4000)
	//	}
}

func stop() {
	//	clearInterval(tim1);
	//	clearInterval(tim2);
	//	tim1 = tim2 = undefined;
}
