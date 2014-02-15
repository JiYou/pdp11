package pdp11

import "fmt"

var rs = [...]string{"R0", "R1", "R2", "R3", "R4", "R5", "SP", "PC"}

const (
	flagD = 1 << 0
	flagS = 1 << 1
	flagO = 1 << 2
	flagR = 1 << 3
	flagN = 1 << 4
)

var disasmtable = []struct {
	inst, arg uint16
	msg       string
	flag      uint
	b         bool
}{
	{0077700, 0005000, "CLR", flagD, true},
	{0077700, 0005100, "COM", flagD, true},
	{0077700, 0005200, "INC", flagD, true},
	{0077700, 0005300, "DEC", flagD, true},
	{0077700, 0005400, "NEG", flagD, true},
	{0077700, 0005700, "TST", flagD, true},
	{0077700, 0006200, "ASR", flagD, true},
	{0077700, 0006300, "ASL", flagD, true},
	{0077700, 0006000, "ROR", flagD, true},
	{0077700, 0006100, "ROL", flagD, true},
	{0177700, 0000300, "SWAB", flagD, false},
	{0077700, 0005500, "ADC", flagD, true},
	{0077700, 0005600, "SBC", flagD, true},
	{0177700, 0006700, "SXT", flagD, false},
	{0070000, 0010000, "MOV", flagS | flagD, true},
	{0070000, 0020000, "CMP", flagS | flagD, true},
	{0170000, 0060000, "ADD", flagS | flagD, false},
	{0170000, 0160000, "SUB", flagS | flagD, false},
	{0070000, 0030000, "BIT", flagS | flagD, true},
	{0070000, 0040000, "BIC", flagS | flagD, true},
	{0070000, 0050000, "BIS", flagS | flagD, true},
	{0177000, 0070000, "MUL", flagR | flagD, false},
	{0177000, 0071000, "DIV", flagR | flagD, false},
	{0177000, 0072000, "ASH", flagR | flagD, false},
	{0177000, 0073000, "ASHC", flagR | flagD, false},
	{0177400, 0000400, "BR", flagO, false},
	{0177400, 0001000, "BNE", flagO, false},
	{0177400, 0001400, "BEQ", flagO, false},
	{0177400, 0100000, "BPL", flagO, false},
	{0177400, 0100400, "BMI", flagO, false},
	{0177400, 0101000, "BHI", flagO, false},
	{0177400, 0101400, "BLOS", flagO, false},
	{0177400, 0102000, "BVC", flagO, false},
	{0177400, 0102400, "BVS", flagO, false},
	{0177400, 0103000, "BCC", flagO, false},
	{0177400, 0103400, "BCS", flagO, false},
	{0177400, 0002000, "BGE", flagO, false},
	{0177400, 0002400, "BLT", flagO, false},
	{0177400, 0003000, "BGT", flagO, false},
	{0177400, 0003400, "BLE", flagO, false},
	{0177700, 0000100, "JMP", flagD, false},
	{0177000, 0004000, "JSR", flagR | flagD, false},
	{0177770, 0000200, "RTS", flagR, false},
	{0177777, 0006400, "MARK", 0, false},
	{0177000, 0077000, "SOB", flagR | flagO, false},
	{0177777, 0000005, "RESET", 0, false},
	{0177700, 0006500, "MFPI", flagD, false},
	{0177700, 0006600, "MTPI", flagD, false},
	{0177777, 0000001, "WAIT", 0, false},
	{0177777, 0000002, "RTI", 0, false},
	{0177777, 0000006, "RTT", 0, false},
	{0177400, 0104000, "EMT", flagN, false},
	{0177400, 0104400, "TRAP", flagN, false},
	{0177777, 0000003, "BPT", 0, false},
	{0177777, 0000004, "IOT", 0, false},
}

func (c *cpu) disasmaddr(m uint16, a uint18) string {
	if (m & 7) == 7 {
		switch m {
		case 027:
			a += 2
			return fmt.Sprintf("$%06o", c.unibus.read16(a))
		case 037:
			a += 2
			return fmt.Sprintf("*%06o", c.unibus.read16(a))
		case 067:
			a += 2
			return fmt.Sprintf("*%06o", (a+2+uint18(c.unibus.read16(a)))&0xFFFF)
		case 077:
			return fmt.Sprintf("**%06o", (a+2+uint18(c.unibus.read16(a)))&0xFFFF)
		}
	}
	r := rs[m&7]
	switch m & 070 {
	case 000:
		return r
	case 010:
		return "(" + r + ")"
	case 020:
		return "(" + r + ")+"
	case 030:
		return "*(" + r + ")+"
	case 040:
		return "-(" + r + ")"
	case 050:
		return "*-(" + r + ")"
	case 060:
		a += 2
		return fmt.Sprintf("%06o (%s)", c.unibus.read16(a), r)
	case 070:
		a += 2
		return fmt.Sprintf("*%06o (%s)", c.unibus.read16(a), r)
	}
	panic(fmt.Sprintf("disasmaddr: unknown addressing mode, register %v, mode %o", r, m&070))
}

func (c *cpu) disasm(a uint18) string {
	ins := c.unibus.read16(a)
	msg := "???"
	l := disasmtable[0]
	for i := 0; i < len(disasmtable); i++ {
		l = disasmtable[i]
		if (ins & l.inst) == l.arg {
			msg = l.msg
			break
		}
	}
	if msg == "???" {
		return msg
	}
	if l.b && (ins&0100000 == 0100000) {
		msg += "B"
	}
	s := (ins & 07700) >> 6
	d := ins & 077
	o := byte(ins & 0377)
	switch l.flag {
	case flagS | flagD:
		msg += " " + c.disasmaddr(s, a) + ","
		fallthrough
	case flagD:
		msg += " " + c.disasmaddr(d, a)
	case flagR | flagO:
		msg += " " + rs[(ins&0700)>>6] + ","
		o &= 077
		fallthrough
	case flagO:
		if o&0x80 == 0x80 {
			msg += fmt.Sprintf(" -%#o", (2 * ((0xFF ^ o) + 1)))
		} else {
			msg += fmt.Sprintf(" +%#o", (2 * o))
		}
	case flagR|flagD:
		msg += " " + rs[(ins&0700)>>6] + ", " + c.disasmaddr(d, a)
	case flagR:
		msg += " " + rs[ins&7]
	}
	return msg
}
