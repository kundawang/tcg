package tcg

import (
	"github.com/gdamore/tcell/v3"
)

// pixel colors
const (
	White = 0 // without pixel
	Black = 1 // it will be black on a terminal with light theme, and white on dark terminals
)

var defaultStyle = tcell.StyleDefault.Foreground(tcell.ColorDefault)

// Tcg - tcell graphics object
type Tcg struct {
	mode          PixelMode
	config        tcgConfig
	scrW, scrH    int          // screen or clip of screen width/height in characters
	Width, Height int          // screen or clip of screen width/height in pixels
	TCellScreen   tcell.Screen // tcell object for keyboard interactions, or low level interactions with terminal screen
	Buf           Buffer       // buffer presents current screen
	style         tcell.Style
}

// New - get new object with tcell inside
func New(mode PixelMode, opts ...Opt) (*Tcg, error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}

	if err := screen.Init(); err != nil {
		return nil, err
	}

	scrW, scrH := screen.Size()
	config := tcgConfig{width: scrW, height: scrH, style: defaultStyle}
	for _, optFn := range opts {
		if err := optFn(&config); err != nil {
			screen.Fini()
			return nil, err
		}
	}

	width := scrW * mode.Width()
	height := scrH * mode.Height()

	result := Tcg{
		mode:        mode,
		config:      config,
		scrW:        scrW,
		scrH:        scrH,
		Width:       width,
		Height:      height,
		TCellScreen: screen,
		style:       config.style,
	}
	result.applyClip()

	return &result, nil
}

func (tg *Tcg) applyClip() {
	if tg.config.clip.width > 0 && tg.config.clip.height > 0 {
		tg.scrW = tg.config.clip.width
		tg.scrH = tg.config.clip.height
		tg.Width = tg.config.clip.width * tg.mode.Width()
		tg.Height = tg.config.clip.height * tg.mode.Height()
	}

	tg.Buf = NewBuffer(tg.Width, tg.Height)
}

// Show - update screen
// it updates the screen with the current buffer content and can erase any text printed before by PrintStr or PrintStrStyle.
// finalisers - should be used to apply additional changes to the screen after pixels are updated, for example, to print some text on the screen.
func (tg *Tcg) Show(finalisers ...func(tg *Tcg)) {
	tg.updateScreen()
	for _, finaliser := range finalisers {
		finaliser(tg)
	}
	tg.TCellScreen.Show()
}

func (tg *Tcg) updateScreen() {
	for x := 0; x < tg.scrW; x++ {
		for y := 0; y < tg.scrH; y++ {
			tg.updateCell(x, y)
		}
	}
}

// Cell - terminal cell in characters, it is used for partial screen updates
type Cell struct {
	X, Y int
}

// Mode - pixel mode of the screen, it defines how many pixels one terminal cell contains.
func (tg *Tcg) Mode() PixelMode {
	return tg.mode
}

// CellRune - symbol that represents the given terminal cell for the current
// buffer content and pixel mode. It is the exact rune that updateCell would
// put on the screen, and is useful for tests and for computing the cells that
// really changed. Cells outside the screen return the empty cell symbol.
func (tg *Tcg) CellRune(x, y int) rune {
	mode := tg.mode
	blockW, blockH := mode.Width(), mode.Height()

	// out of the visible screen: clamp to a valid (empty) area
	if x < 0 || x >= tg.scrW || y < 0 || y >= tg.scrH {
		return mode.charMapping[0]
	}
	// a partially filled last cell is padded with transparent pixels by
	// getPixelsBlock, which stops at the buffer edge
	charIndex := tg.Buf.getPixelsBlock(x*blockW, y*blockH, blockW, blockH)
	return mode.charMapping[charIndex]
}

// updateCell - update one terminal cell from the pixel buffer
func (tg *Tcg) updateCell(x, y int) {
	if x < 0 || x >= tg.scrW || y < 0 || y >= tg.scrH {
		return
	}

	tg.TCellScreen.SetContent(tg.config.clip.x+x, tg.config.clip.y+y, tg.CellRune(x, y), nil, tg.style)
}

// UpdateCells - update only the given terminal cells from the pixel buffer.
// It does not call Show, so it can be combined with other cell updates.
// Cells outside the screen are ignored, duplicates are harmless.
// This is useful for sprite animations, where only the cells touched by the
// sprite (or by its previous position) need to be repainted.
func (tg *Tcg) UpdateCells(cells ...Cell) {
	for _, cell := range cells {
		tg.updateCell(cell.X, cell.Y)
	}
}

// ShowCells - update only the given terminal cells from the pixel buffer and show the screen.
func (tg *Tcg) ShowCells(cells ...Cell) {
	tg.UpdateCells(cells...)
	tg.TCellScreen.Show()
}

// Finish application
func (tg *Tcg) Finish() {
	tg.TCellScreen.Fini()
}

// SetClip - set new clip of screen, this allows to limit the area of the screen that will be used for drawing.
// x, y - is in screen character coordinates, not pixels.
// width, height - is in screen character coordinates, not pixels.
// It can be run any time, but clearing internal buffer screen.
func (tg *Tcg) SetClip(x, y, width, height int) error {
	if err := WithClip(x, y, width, height)(&tg.config); err != nil {
		return err
	}
	tg.applyClip()
	return nil
}

// SetClipCenter - set new clip in the center of screen
func (tg *Tcg) SetClipCenter(width, height int) error {
	if err := WithClipCenter(width, height)(&tg.config); err != nil {
		return err
	}
	tg.applyClip()
	return nil
}

// PrintStr - print string on screen, with white on black style
// string don't save in the buffer of pixels!
// x, y - is in screen character coordinates, not pixels.
// Also x/y coordinates is not use Clip of the screen, it's always absolute.
func (tg *Tcg) PrintStr(x, y int, str string) {
	tg.PrintStrStyled(x, y, str, tg.style)
}

// PrintStrStyled - print string on screen
// see the PrintStr about restrictions
func (tg *Tcg) PrintStrStyled(x, y int, str string, style tcell.Style) {
	tg.TCellScreen.PutStrStyled(x, y, str, style)
}

// ScreenSize - returns terminal screen size in chars (width, height)
func (tg *Tcg) ScreenSize() (int, int) {
	return tg.TCellScreen.Size()
}
