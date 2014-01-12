package pdp11

type TM11 struct {
	MTS   uint16
	MTC   uint16
	MTBRC uint16
	MTCMA uint16
	MTD   uint16
	MTRD  uint16
}

func (t *TM11) read16(a uint18) uint16 {
	switch a {

	default:
		panic("tm11: invalid read")
	}
}

func (t *TM11) write16(a uint18, v uint16) {
	switch a {

	default:
		panic("tm11: invalid write")
	}
}
