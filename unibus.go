package pdp11

var memory [128 * 1024]int // word addressing

type Unibus struct {
	LKS  int
	cpu  *KB11
	rk   *RK05 // drive 0
	cons *Console
}

func (u *Unibus) physread16(a int) int {
	switch {
	case a&1 == 1:
		panic(trap{INTBUS, "read from odd address " + ostr(a, 6)})
	case a < 0760000:
		return memory[a>>1]
	case a == 0777546:
		return u.LKS
	case a == 0777570:
		return 0173030
	case a == 0777572:
		return int(u.cpu.SR0)
	case a == 0777576:
		return int(u.cpu.SR2)
	case a == 0777776:
		return int(u.cpu.PS)
	case a&0777770 == 0777560:
		return u.cons.consread16(a)
	case a&0777760 == 0777400:
		return u.rk.rkread16(a)
	case a&0777600 == 0772200 || (a&0777600) == 0777600:
		return mmuread16(a)
	case a == 0776000:
		panic("lolwut")
	default:
		panic(trap{INTBUS, "read from invalid address " + ostr(a, 6)})
	}
}

func (u *Unibus) physread8(a int) int {
	val := u.physread16(a & ^1)
	if a&1 != 0 {
		return val >> 8
	}
	return val & 0xFF
}

func (u *Unibus) physwrite8(a, v int) {
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
			u.physwrite16(a&^1, (u.physread16(a)&0xFF)|(v&0xFF)<<8)
		} else {
			u.physwrite16(a&^1, (u.physread16(a)&0xFF00)|(v&0xFF))
		}
	}
}

func (u *Unibus) physwrite16(a, v int) {
	if a%1 != 0 {
		panic(trap{INTBUS, "write to odd address " + ostr(a, 6)})
	}
	if a < 0760000 {
		memory[a>>1] = v
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
			prevuser = false
			break
		case 3:
			prevuser = true
			break
		default:
			panic("invalid mode")
		}
		u.cpu.PS = uint16(v)
	} else if a == 0777546 {
		u.LKS = v
	} else if a == 0777572 {
		u.cpu.SR0 = uint16(v)
	} else if (a & 0777770) == 0777560 {
		u.cons.conswrite16(a, v)
	} else if (a & 0777700) == 0777400 {
		u.rk.rkwrite16(a, v)
	} else if (a&0777600) == 0772200 || (a&0777600) == 0777600 {
		mmuwrite16(a, v)
	} else {
		panic(trap{INTBUS, "write to invalid address " + ostr(a, 6)})
	}
}
