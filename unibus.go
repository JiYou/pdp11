package pdp11

import "fmt"

type unibus struct {
	Memory [128 * 1024]uint16
	LKS    uint16
	cpu    *cpu
	rk     RK05 // drive 0
	cons   Console
}

// uint18 represents a unibus 18 bit physical address
type uint18 uint32

func (u *unibus) physread16(a uint18) uint16 {
	switch {
	case a&1 == 1:
		panic(trap{INTBUS, fmt.Sprintf("read from odd address %06o", a)})
	case a < 0760000:
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
		return u.cpu.PS
	case a&0777770 == 0777560:
		return uint16(u.cons.consread16(a))
	case a&0777760 == 0777400:
		return uint16(u.rk.rkread16(a))
	case a&0777600 == 0772200 || (a&0777600) == 0777600:
		return u.cpu.mmu.mmuread16(a)
	case a == 0776000:
		panic("lolwut")
	default:
		panic(trap{INTBUS, fmt.Sprintf("read from invalid address %06o", a)})
	}
}

func (u *unibus) physread8(a uint18) uint16 {
	val := u.physread16(a & ^uint18(1))
	if a&1 != 0 {
		return val >> 8
	}
	return val & 0xFF
}

func (u *unibus) physwrite8(a uint18, v uint16) {
	if a < 0760000 {
		if a&1 == 1 {
			u.Memory[a>>1] &= 0xFF
			u.Memory[a>>1] |= v & 0xFF << 8
		} else {
			u.Memory[a>>1] &= 0xFF00
			u.Memory[a>>1] |= v & 0xFF
		}
	} else {
		if a&1 == 1 {
			u.physwrite16(a&^1, (u.physread16(a)&0xFF)|(v&0xFF)<<8)
		} else {
			u.physwrite16(a&^1, (u.physread16(a)&0xFF00)|(v&0xFF))
		}
	}
}

func (u *unibus) physwrite16(a uint18, v uint16) {
	if a%1 != 0 {
		panic(trap{INTBUS, fmt.Sprintf("write to odd address %06o", a)})
	}
	if a < 0760000 {
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
		u.cpu.PS = v
	} else if a == 0777546 {
		u.LKS = v
	} else if a == 0777572 {
		u.cpu.mmu.SR0 = v
	} else if (a & 0777770) == 0777560 {
		u.cons.conswrite16(a, int(v))
	} else if (a & 0777700) == 0777400 {
		u.rk.rkwrite16(a, int(v))
	} else if (a&0777600) == 0772200 || (a&0777600) == 0777600 {
		u.cpu.mmu.mmuwrite16(a, v)
	} else {
		panic(trap{INTBUS, fmt.Sprintf("write to invalid address %06o", a)})
	}
}
