package pdp11

import "fmt"

const MEMSIZE = 0760000

type unibus struct {
	Memory [MEMSIZE >> 1]uint16
	LKS    uint16
	cpu    *cpu
	rk     RK11 // drive 0
	cons   Console
}

// uint18 represents a unibus 18 bit physical address
type uint18 uint32

func (u *unibus) Reset() {
	for i := uint18(0); int(i) < len(u.Memory); i++ {
		u.write16(i, 0)
	}
}

func (u *unibus) read16(a uint18) uint16 {
	switch {
	case a&1 == 1:
		panic(trap{intBUS, fmt.Sprintf("read from odd address %06o", a)})
	case a < MEMSIZE:
		return u.Memory[a>>1]
	case a == 0777546:
		return u.LKS
	case a == 0777570:
		return 0173030
	case a == 0777572:
		return u.cpu.mmu.SR0
	case a == 0777576:
		return u.cpu.mmu.SR2
	case a == 0777776:
		return uint16(u.cpu.PS)
	case a&0777770 == 0777560:
		return uint16(u.cons.consread16(a))
	case a&0777760 == 0777400:
		return u.rk.read16(a)
	case a&0777600 == 0772200 || (a&0777600) == 0777600:
		return u.cpu.mmu.read16(a)
	case a == 0776000:
		panic("lolwut")
	default:
		panic(trap{intBUS, fmt.Sprintf("read from invalid address %06o", a)})
	}
}

func (u *unibus) read8(a uint18) uint16 {
	val := u.read16(a & ^uint18(1))
	if a&1 != 0 {
		return val >> 8
	}
	return val & 0xFF
}

func (u *unibus) write8(a uint18, v uint16) {
	if a < MEMSIZE {
		if a&1 == 1 {
			u.Memory[a>>1] &= 0xFF
			u.Memory[a>>1] |= v & 0xFF << 8
		} else {
			u.Memory[a>>1] &= 0xFF00
			u.Memory[a>>1] |= v & 0xFF
		}
	} else {
		if a&1 == 1 {
			u.write16(a&^1, (u.read16(a)&0xFF)|(v&0xFF)<<8)
		} else {
			u.write16(a&^1, (u.read16(a)&0xFF00)|(v&0xFF))
		}
	}
}

func (u *unibus) write16(a uint18, v uint16) {
	if a%1 != 0 {
		panic(trap{intBUS, fmt.Sprintf("write to odd address %06o", a)})
	}
	if a < MEMSIZE {
		u.Memory[a>>1] = v
	} else if a == 0777776 {
		switch v >> 14 {
		case 0:
			u.cpu.switchmode(false)
			break
		case 3:
			u.cpu.switchmode(true)
			break
		default:
			panic("invalid mode")
		}
		switch (v >> 12) & 3 {
		case 0:
			u.cpu.prevuser = false
			break
		case 3:
			u.cpu.prevuser = true
			break
		default:
			panic("invalid mode")
		}
		u.cpu.PS = PSW(v)
	} else if a == 0777546 {
		u.LKS = v
	} else if a == 0777572 {
		u.cpu.mmu.SR0 = v
	} else if (a & 0777770) == 0777560 {
		u.cons.conswrite16(a, int(v))
	} else if (a & 0777700) == 0777400 {
		u.rk.write16(a, v)
	} else if (a&0777600) == 0772200 || (a&0777600) == 0777600 {
		u.cpu.mmu.write16(a, v)
	} else {
		panic(trap{intBUS, fmt.Sprintf("write to invalid address %06o", a)})
	}
}
