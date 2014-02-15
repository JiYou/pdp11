package pdp11

import (
	"fmt"
	"time"
)

func (c *cpu) timing(a uint18) time.Duration {
	ins := c.unibus.read16(a)
	l := disasmtable[0]
	for i := 0; i < len(disasmtable); i++ {
		l = disasmtable[i]
		if (ins & l.inst) == l.arg {
			goto found
		}
	}
	c.printstate()
	panic(fmt.Sprintf("timing: cannot decode instruction %06o at %06o", ins, a))

found:
	msg := l.msg
	var b bool
	if l.b && (ins&0100000 == 0100000) {
		b = true
	}
	sm := (ins & 07000) >> 9
	dm := (ins & 070) >> 3
	switch msg {
	case "ADD", "CMP", "BIT", "BIC", "BIS":
		switch dm {
		case 0:
			if sm == 0 {
				return srcTime(sm) + dstTime(dm, b) + 990*time.Nanosecond
			}
			return srcTime(sm) + dstTime(dm, b) + 1600*time.Nanosecond
		default:
			return srcTime(sm) + dstTime(dm, b) + 1760*time.Nanosecond
		}
	case "SUB":
		switch dm {
		case 0:
			if sm == 0 {
				return srcTime(sm) + dstTime(dm, b) + 990*time.Nanosecond
			}
			return srcTime(sm) + dstTime(dm, b) + 1600*time.Nanosecond
		default:
			return srcTime(sm) + dstTime(dm, b) + 1900*time.Nanosecond
		}
	case "XOR":
		switch dm {
		case 0:
			if sm == 0 {
				return srcTime(sm) + dstTime(dm, b) + 990*time.Nanosecond
			}
			panic("XOR timing")
		default:
			return srcTime(sm) + dstTime(dm, b) + 1760*time.Nanosecond
		}
	case "MOV":
		switch dm {
		case 0:
			if sm == 0 {
				return srcTime(sm) + 900*time.Nanosecond
			}
			return srcTime(sm) + 1460*time.Nanosecond
		case 1, 2, 4:
			return srcTime(sm) + 2420*time.Nanosecond
		case 3, 5:
			return srcTime(sm) + 3180*time.Nanosecond
		case 6:
			if sm == 0 {
				return srcTime(sm) + 2840*time.Nanosecond
			}
			return srcTime(sm) + 3180*time.Nanosecond
		case 7:
			if sm == 0 {
				return srcTime(sm) + 3680*time.Nanosecond
			}
			return srcTime(sm) + 4020*time.Nanosecond
		}
	case "CLR", "COM", "NEG", "INC", "DEC", "ADC", "SBC", "TST", "ROL", "ASL", "SWAB":
		if dm == 0 {
			return dstTime(dm, b) + 990*time.Nanosecond
		}
		return dstTime(dm, b) + 1770*time.Nanosecond
	case "ROR", "ASR":
		if dm == 0 {
			return dstTime(dm, b) + 1250*time.Nanosecond
		}
		return dstTime(dm, b) + 2060*time.Nanosecond
	case "SXT":
		if dm == 0 {
			return dstTime(dm, b) + 900*time.Nanosecond
		}
		return dstTime(dm, b) + 1770*time.Nanosecond
	case "BR", "BNE", "BEQ", "BPL", "BMI", "BVC", "BVS", "BCC", "BCS", "BGE", "BLT", "BGT", "BLE", "BHI", "BLOS", "BHIS", "BLO":
		return ((1760 + 1400) / 2) * time.Nanosecond // assume equal branch percentages
	case "SOB":
		return ((2360 + 2040) / 2) * time.Nanosecond // assume equal branch percentages
	case "MFPI":
		return 3740 * time.Nanosecond
	case "MTPI":
		return 3680 * time.Nanosecond
	case "JMP":
		switch dm {
		case 1:
			return 1800 * time.Nanosecond
		case 2:
			return 2100 * time.Nanosecond
		case 3:
			return 2300 * time.Nanosecond
		case 4:
			return 1900 * time.Nanosecond
		case 5:
			return 2300 * time.Nanosecond
		case 6:
			return 2360 * time.Nanosecond
		case 7:
			return 2920 * time.Nanosecond
		}
	case "JSR":
		switch dm {
		case 1:
			return 2940 * time.Nanosecond
		case 2:
			return 3240 * time.Nanosecond
		case 3:
			return 3440 * time.Nanosecond
		case 4:
			return 3040 * time.Nanosecond
		case 5:
			return 3440 * time.Nanosecond
		case 6:
			return 3500 * time.Nanosecond
		case 7:
			return 4060 * time.Nanosecond
		}
	case "RTS":
		return 2420 * time.Nanosecond
	case "MARK":
		return 2560 * time.Nanosecond
	case "RTI", "RTT":
		return 2920 * time.Nanosecond
	case "HALT":
		return 2420 * time.Nanosecond
	case "WAIT":
		return 2240 * time.Nanosecond
	case "RESET":
		return 80 * time.Millisecond
	case "IOT", "EMT", "TRAP", "BPT":
		return 5800 * time.Nanosecond
	case "MUL":
		return 8880 * time.Nanosecond
	case "DIV":
		return 11300 * time.Nanosecond
	case "ASH":
		return 2580 * time.Nanosecond // TODO(dfc) add shifts
	case "ASHC":
		return 3260 * time.Nanosecond // TODO(dfc) add shifts
	case "FP":
		return 0 // ignore
	}
	c.printstate()
	panic(fmt.Sprintf("timing: cannot time instruction %06o at %06o", ins, a))
}

func srcTime(mode uint16) time.Duration {
	switch mode {
	case 0:
		return 0
	case 1:
		return 780 * time.Nanosecond
	case 2:
		return 840 * time.Nanosecond
	case 3:
		return 1740 * time.Nanosecond
	case 4:
		return 840 * time.Nanosecond
	case 5:
		return 1740 * time.Nanosecond
	case 6:
		return 1460 * time.Nanosecond
	case 7:
		return 2360 * time.Nanosecond
	}
	panic(fmt.Sprintf("srcTime: unknown mode %d", mode))
}

func dstTime(mode uint16, b bool) time.Duration {
	switch mode {
	case 0:
		return 0
	case 1:
		if b {
			return 900 * time.Nanosecond
		}
		return 780 * time.Nanosecond
	case 2:
		if b {
			return 900 * time.Nanosecond
		}
		return 780 * time.Nanosecond
	case 3:
		if b {
			return 1800 * time.Nanosecond
		}
		return 1740 * time.Nanosecond
	case 4:
		if b {
			return 900 * time.Nanosecond
		}
		return 780 * time.Nanosecond
	case 5:
		if b {
			return 1800 * time.Nanosecond
		}
		return 1740 * time.Nanosecond
	case 6:
		if b {
			return 1740 * time.Nanosecond
		}
		return 1460 * time.Nanosecond
	case 7:
		if b {
			return 2640 * time.Nanosecond
		}
		return 2360 * time.Nanosecond
	}
	panic(fmt.Sprintf("dstTime: unknown mode %d", mode))
}
