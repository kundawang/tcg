package tcg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTcgMode(t *testing.T) {
	tg := Tcg{mode: Mode2x3}
	assert.Equal(t, Mode2x3, tg.Mode())
	assert.Equal(t, 2, tg.Mode().Width())
	assert.Equal(t, 3, tg.Mode().Height())
}

func TestTcgCellRune(t *testing.T) {
	tg := Tcg{
		mode:   Mode1x1Simple,
		scrW:   2,
		scrH:   2,
		Buf:    MustNewBufferFromStrings([]string{"*.", ".*"}),
		config: tcgConfig{},
	}

	assert.Equal(t, '*', tg.CellRune(0, 0))
	assert.Equal(t, '.', tg.CellRune(1, 0))
	assert.Equal(t, '.', tg.CellRune(0, 1))
	assert.Equal(t, '*', tg.CellRune(1, 1))

	// outside of the screen returns the empty cell rune
	assert.Equal(t, '.', tg.CellRune(-1, 0))
	assert.Equal(t, '.', tg.CellRune(0, -1))
	assert.Equal(t, '.', tg.CellRune(2, 0))
	assert.Equal(t, '.', tg.CellRune(0, 2))
}

func TestTcgCellRuneMultiPixelMode(t *testing.T) {
	tg := Tcg{
		mode:   Mode2x2,
		scrW:   2,
		scrH:   1,
		Buf:    MustNewBufferFromStrings([]string{"*.", ".*"}),
		config: tcgConfig{},
	}

	// the first cell contains the pixels "*. / .*" -> index 0b1001 = 9
	expected := tg.mode.charMapping[0b1001]
	assert.Equal(t, expected, tg.CellRune(0, 0))

	// the second cell is outside the buffer content, all pixels are transparent
	assert.Equal(t, tg.mode.charMapping[0], tg.CellRune(1, 0))
}

func TestTcgUpdateCellsOutOfRange(t *testing.T) {
	// updateCell must ignore cells outside of the visible screen without
	// touching the (nil) tcell screen
	tg := Tcg{
		mode:   Mode1x1,
		scrW:   2,
		scrH:   2,
		Buf:    MustNewBufferFromStrings([]string{"..", ".."}),
		config: tcgConfig{},
	}

	assert.NotPanics(t, func() {
		tg.updateCell(-1, 0)
		tg.updateCell(0, -1)
		tg.updateCell(2, 0)
		tg.updateCell(0, 2)
	})
}
