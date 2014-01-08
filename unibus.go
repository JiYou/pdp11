package pdp11

var memory [128 * 1024]int // word addressing

type Unibus struct {
	LKS int
	cpu *KB11
}

func (k *KB11) physread16(a int) int {
	switch {
	case a&1 == 1:
		panic(trap{INTBUS, "read from odd address " + ostr(a, 6)})
	case a < 0760000:
		return memory[a>>1]
	case a == 0777546:
		return k.unibus.LKS
	case a == 0777570:
		return 0173030
	case a == 0777572:
		return k.SR0
	case a == 0777576:
		return k.SR2
	case a == 0777776:
		return k.PS
	case a&0777770 == 0777560:
		return consread16(a)
	case a&0777760 == 0777400:
		return rkread16(a)
	case a&0777600 == 0772200 || (a&0777600) == 0777600:
		return mmuread16(a)
	case a == 0776000:
		panic("lolwut")
	default:
		panic(trap{INTBUS, "read from invalid address " + ostr(a, 6)})
	}
}

func (k *KB11) physread8(a int) int {
	val := k.physread16(a & ^1)
	if a&1 != 0 {
		return val >> 8
	}
	return val & 0xFF
}

func (k *KB11) physwrite8(a, v int) {
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
			k.physwrite16(a&^1, (k.physread16(a)&0xFF)|(v&0xFF)<<8)
		} else {
			k.physwrite16(a&^1, (k.physread16(a)&0xFF00)|(v&0xFF))
		}
	}
}

func (k *KB11) physwrite16(a, v int) {
	if a%1 != 0 {
		panic(trap{INTBUS, "write to odd address " + ostr(a, 6)})
	}
	if a < 0760000 {
		memory[a>>1] = v
	} else if a == 0777776 {
		switch v >> 14 {
		case 0:
			k.switchmode(false)
			break
		case 3:
			k.switchmode(true)
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
		k.PS = v
	} else if a == 0777546 {
		k.unibus.LKS = v
	} else if a == 0777572 {
		k.SR0 = v
	} else if (a & 0777770) == 0777560 {
		conswrite16(a, v)
	} else if (a & 0777700) == 0777400 {
		rkwrite16(a, v)
	} else if (a&0777600) == 0772200 || (a&0777600) == 0777600 {
		mmuwrite16(a, v)
	} else {
		panic(trap{INTBUS, "write to invalid address " + ostr(a, 6)})
	}
}
