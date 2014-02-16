package pdp11

import "fmt"

const DEBUG_MMU = false

type KT11 struct {
	SR0, SR2 uint16
	cpu      *cpu
	pages    [16]page
}

type page struct {
	par, pdr uint16
}

func (p *page) read() bool   { return p.pdr&2 == 2 }
func (p *page) write() bool  { return p.pdr&6 == 6 }
func (p *page) ed() bool     { return p.pdr&8 == 8 }
func (p *page) addr() uint16 { return p.par & 07777 }
func (p *page) len() uint16  { return (p.pdr >> 8) & 0x7f }

func (m *KT11) read16(a uint18) uint16 {
	i := ((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		return m.pages[i].pdr
	}
	if (a >= 0772340) && (a < 0772360) {
		return m.pages[i].par
	}
	if (a >= 0777600) && (a < 0777620) {
		return m.pages[i+8].pdr
	}
	if (a >= 0777640) && (a < 0777660) {
		return m.pages[i+8].par
	}
	panic(trap{intBUS, fmt.Sprintf("invalid read from %06o", a)})
}

func (m *KT11) write16(a uint18, v uint16) {
	i := ((a & 017) >> 1)
	if (a >= 0772300) && (a < 0772320) {
		m.pages[i].pdr = v
		return
	}
	if (a >= 0772340) && (a < 0772360) {
		m.pages[i].par = v
		return
	}
	if (a >= 0777600) && (a < 0777620) {
		m.pages[i+8].pdr = v
		return
	}
	if (a >= 0777640) && (a < 0777660) {
		m.pages[i+8].par = v
		return
	}
	panic(trap{intBUS, fmt.Sprintf("write to invalid address %06o", a)})
}

func (m *KT11) mmuEnabled() bool  { return m.SR0&1 == 1 }
func (m *KT11) mmuDisabled() bool { return m.SR0&1 == 0 }

func (m *KT11) decode(a uint16, w, user bool) (addr uint18) {
	if m.mmuDisabled() {
		aa := uint18(a)
		if aa >= 0170000 {
			aa += 0600000
		}
		if DEBUG_MMU {
			fmt.Printf("decode: fast %06o -> %06o\n", a, aa)
		}
		return aa
	}
	offset := a >> 13
	if user {
		offset += 8
	}
	p := m.pages[offset]
	if w && !p.write() {
		m.SR0 = (1 << 13) | 1
		m.SR0 |= a >> 12 & ^uint16(1)
		if user {
			m.SR0 |= (1 << 5) | (1 << 6)
		}
		m.SR2 = m.cpu.pc
		panic(trap{intFAULT, fmt.Sprintf("write to read-only page %06o", a)})
	}
	if !p.read() {
		m.SR0 = (1 << 15) | 1
		m.SR0 |= (a >> 12) & ^uint16(1)
		if user {
			m.SR0 |= (1 << 5) | (1 << 6)
		}
		m.SR2 = m.cpu.pc
		panic(trap{intFAULT, fmt.Sprintf("read from no-access page %06o", a)})
	}
	block := (a >> 6) & 0177
	disp := uint18(a & 077)
	if p.ed() && block < p.len() || !p.ed() && block > p.len() {
		//if(p.ed ? (block < p.len) : (block > p.len)) {
		m.SR0 = (1 << 14) | 1
		m.SR0 |= (a >> 12) & ^uint16(1)
		if user {
			m.SR0 |= (1 << 5) | (1 << 6)
		}
		m.SR2 = m.cpu.pc
		panic(trap{intFAULT, fmt.Sprintf("page length exceeded, address %06o (block %03o) is beyond %03o", a, block, p.len())})
	}
	if w {
		p.pdr |= 1 << 6
	}
	aa := ((uint18(block) + uint18(p.addr())) << 6) + disp
	if DEBUG_MMU {
		fmt.Printf("decode: slow %06o -> %06o\n", a, aa)
	}
	return aa
}
