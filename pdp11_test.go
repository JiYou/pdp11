package pdp11

import "testing"
import "time"

func TestXOR(t *testing.T) {
	for _, tt := range []struct {
		x, y, z int
	}{
		{0, 0, 0},
		{1, 0, 1},
		{0, 1, 1},
		{1, 1, 0},
	} {
		got := xor(tt.x, tt.y)
		if got != tt.z {
			t.Errorf("xor(%d, %d) = %d; want %d", tt.x, tt.y, got, tt.z)
		}
	}
}

const N = 4 * 1000 * 1000

var pdpTests = []struct {
	input  string
	cycles int
}{
	//	{"STTY -LCASE\n", N},
	{"\ndate\n", N},    // processor loops
	{"\nls /bin\n", N}, // read from odd address
	{"\nwho\n", N},     // read from no-access page 01002
	{"\ncat /etc/passwd\n", N},
	{`
ed TEST.C
a
main() {
printf("Hello, world!\n");
}
.
w
q
cc test
a.out
`, 2 * N},
}

func TestPDP(t *testing.T) {
	for _, tt := range pdpTests {
		cpu := New()
		go func() {
			c := cpu.Input
			c <- 'u'
			c <- 'n'
			c <- 'i'
			c <- 'x'
			c <- '\n'
			time.Sleep(10 * time.Microsecond)
			for _, c := range tt.input {
				cpu.Input <- uint8(c)
			}
		}()
		for i := 0; i < tt.cycles; i++ {
			cpu.Step()
		}
	}
}
