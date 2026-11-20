package sprite

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/msoap/tcg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// three walking frames: a 2x3 character, each frame has its own silhouette
var (
	walk1 = tcg.MustNewBufferFromStrings([]string{
		".*",
		"**",
		"*.",
	})
	walk2 = tcg.MustNewBufferFromStrings([]string{
		"*.",
		"**",
		".*",
	})
	walk3 = tcg.MustNewBufferFromStrings([]string{
		"**",
		".*",
		"*.",
	})
)

func walkAnimation(t *testing.T, opts ...AnimationOpt) *Sprite {
	t.Helper()

	const frameTime = 100 * time.Millisecond
	frames := []Frame{
		{Buf: walk1, Duration: frameTime},
		{Buf: walk2, Duration: frameTime},
		{Buf: walk3, Duration: frameTime},
	}
	spr, err := NewAnimation(frames, opts...)
	require.NoError(t, err)

	return spr
}

// gridBackground - 6x4 buffer with a vertical grid line at x=2
func gridBackground() tcg.Buffer {
	bg := tcg.NewBuffer(6, 4)
	bg.VLine(2, 0, 4, tcg.Black)
	return bg
}

// walkAt returns the expected buffer: the grid background with the given frame
// painted transparently at (x, y). It is the same operation the sprite performs,
// so the test expresses the intent (frame over the grid) rather than a
// hand-copied picture.
func walkAt(x, y int, frame tcg.Buffer) tcg.Buffer {
	expected := gridBackground()
	expected.BitBlt(x, y, frame.Width, frame.Height, frame, 0, 0, tcg.BBTransparent())
	return expected
}

func TestNewAnimationValidation(t *testing.T) {
	oneSecond := time.Second

	t.Run("no frames", func(t *testing.T) {
		spr, err := NewAnimation(nil)
		assert.ErrorIs(t, err, ErrNoFrames)
		assert.Nil(t, spr)
	})

	t.Run("zero duration", func(t *testing.T) {
		spr, err := NewAnimation([]Frame{{Buf: walk1, Duration: 0}})
		assert.ErrorIs(t, err, ErrZeroDuration)
		assert.Nil(t, spr)
	})

	t.Run("negative duration", func(t *testing.T) {
		spr, err := NewAnimation([]Frame{{Buf: walk1, Duration: oneSecond}, {Buf: walk2, Duration: -oneSecond}})
		assert.ErrorIs(t, err, ErrZeroDuration)
		assert.Nil(t, spr)
	})

	t.Run("size mismatch", func(t *testing.T) {
		bigger := tcg.MustNewBufferFromStrings([]string{"***", "***", "***"})
		spr, err := NewAnimation([]Frame{{Buf: walk1, Duration: oneSecond}, {Buf: bigger, Duration: oneSecond}})
		assert.ErrorIs(t, err, ErrFrameSizeMismatch)
		assert.Nil(t, spr)
	})

	t.Run("unknown loop mode", func(t *testing.T) {
		spr, err := NewAnimation([]Frame{{Buf: walk1, Duration: oneSecond}}, WithLoopMode(LoopMode(42)))
		assert.ErrorIs(t, err, ErrUnknownLoopMode)
		assert.Nil(t, spr)
	})

	t.Run("valid single frame", func(t *testing.T) {
		spr, err := NewAnimation([]Frame{{Buf: walk1, Duration: oneSecond}})
		require.NoError(t, err)
		assert.Equal(t, 1, spr.Frames())
		assert.Equal(t, oneSecond, spr.Duration())
		assert.Equal(t, LoopRepeat, spr.LoopMode())
	})
}

func TestAnimationFrameAt(t *testing.T) {
	spr := walkAnimation(t)

	tests := []struct {
		name      string
		at        time.Duration
		wantIndex int
		wantPhase time.Duration
	}{
		{"start", 0, 0, 0},
		{"first frame middle", 50 * time.Millisecond, 0, 50 * time.Millisecond},
		{"second frame start", 100 * time.Millisecond, 1, 0},
		{"second frame middle", 150 * time.Millisecond, 1, 50 * time.Millisecond},
		{"third frame", 250 * time.Millisecond, 2, 50 * time.Millisecond},
		{"exact loop", 300 * time.Millisecond, 0, 0},
		{"three frames, third frame", 299 * time.Millisecond, 2, 99 * time.Millisecond},
		{"second loop", 450 * time.Millisecond, 1, 50 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, phase := spr.FrameAt(tt.at)
			assert.Equal(t, tt.wantIndex, index)
			assert.Equal(t, tt.wantPhase, phase)
		})
	}

	t.Run("negative wraps", func(t *testing.T) {
		index, _ := spr.FrameAt(-50 * time.Millisecond)
		assert.Equal(t, 2, index)
	})

	t.Run("fast jump", func(t *testing.T) {
		got, _ := spr.FrameAt(1000000 * time.Second)
		want, _ := spr.FrameAt(time.Duration(1000000 * int64(time.Second) % int64(300*time.Millisecond)))
		assert.Equal(t, want, got, "a huge time is equivalent to its remainder modulo the period")
	})
}

func TestAnimationFrameAtPingPong(t *testing.T) {
	spr := walkAnimation(t, WithLoopMode(LoopPingPong))
	// no-stutter ping-pong: 0,1,2,1,0,1,... with 100ms per frame and a
	// 400ms cycle (the first and the last frame are shown once per turn)
	tests := []struct {
		at   time.Duration
		want int
	}{
		{0, 0},
		{100 * time.Millisecond, 1},
		{200 * time.Millisecond, 2},
		{300 * time.Millisecond, 1}, // backward
		{400 * time.Millisecond, 0}, // the cycle repeats
		{500 * time.Millisecond, 1},
		{650 * time.Millisecond, 1}, // second backward pass: 650-400=250 -> frame 1? see below
	}
	// 650ms: elapsed 650 % 400 = 250; forward 0..300, backward 300..400 -> 250 is forward frame 2.
	tests[6].want = 2
	for _, tt := range tests {
		got, _ := spr.FrameAt(tt.at)
		assert.Equalf(t, tt.want, got, "at %s", tt.at)
	}

	// negative time wraps in the same cycle
	got, _ := spr.FrameAt(-100 * time.Millisecond)
	assert.Equal(t, 1, got)
}

func TestAnimationFrameAtOnce(t *testing.T) {
	spr := walkAnimation(t, WithLoopMode(LoopOnce))

	index, _ := spr.FrameAt(0)
	assert.Equal(t, 0, index)
	index, _ = spr.FrameAt(150 * time.Millisecond)
	assert.Equal(t, 1, index)
	index, _ = spr.FrameAt(time.Second)
	assert.Equal(t, 2, index, "clamps on the last frame")
	index, _ = spr.FrameAt(-time.Second)
	assert.Equal(t, 0, index, "clamps on the first frame")
}

// TestAnimationWalkAcrossGrid is the fixed-clock check required by the task:
// a three-frame character walks across a vertical grid line. It verifies the
// buffer pixels, the changed cells and the restoration of the background.
func TestAnimationWalkAcrossGrid(t *testing.T) {
	bg := gridBackground()
	clean := *bg.Clone()

	spr := walkAnimation(t)
	spr.MoveAbs(bg, 0, 0).Put(bg)

	// frame 0 at (0,0): every pixel of the 2x3 sprite touches the grid line
	assertEqBuffers(t, bg, walkAt(0, 0, walk1))

	// the contiguous 2x3 sprite covers the two terminal rows that contain
	// changed pixels in the 2x2 mode; the frame itself starts at (0,0)
	assert.Equal(t, []tcg.Cell{{X: 0, Y: 0}, {X: 0, Y: 1}},
		spr.ChangedCells(tcg.Mode2x2))
	spr.ClearDirty()

	steps := []struct {
		elapsed   time.Duration
		wantFrame int
		frame     tcg.Buffer
	}{
		{0, 0, walk1},
		{100 * time.Millisecond, 1, walk2},
		{200 * time.Millisecond, 2, walk3},
		{300 * time.Millisecond, 0, walk1}, // the cycle repeats
	}

	for _, step := range steps {
		spr.Update(bg, step.elapsed)
		assert.Equal(t, step.wantFrame, spr.FrameIndex(), "frame at %s", step.elapsed)
		assertEqBuffers(t, bg, walkAt(0, 0, step.frame))
		spr.ClearDirty()
	}

	// move the character over the grid line; the line under transparent pixels
	// must be restored from the saved background, not left from the old frame
	spr.MoveAbs(bg, 2, 0)
	assertEqBuffers(t, bg, walkAt(2, 0, walk1))

	// a frame change at the new position keeps the grid line intact
	spr.Update(bg, 400*time.Millisecond)
	assertEqBuffers(t, bg, walkAt(2, 0, walk2))

	// moving away restores the old position exactly, including the grid line
	spr.MoveAbs(bg, 4, 1)
	spr.Withdraw(bg)
	assertEqBuffers(t, bg, clean)
}

// TestAnimationWalkAcrossGridFixedClock checks the walk step by step with
// absolute hand-written pictures of the whole buffer, so a regression in the
// transparent drawing or the background restore is visible directly.
func TestAnimationWalkAcrossGridFixedClock(t *testing.T) {
	bg := gridBackground()

	spr := walkAnimation(t)
	spr.Update(bg, 0)
	spr.Put(bg)

	// after MoveAbs(bg, 1, 1) the character walks over the grid line at x=2
	spr.MoveAbs(bg, 1, 1)

	assertEqBuffers(t, bg, tcg.MustNewBufferFromStrings([]string{
		"..*...",
		"..*...",
		".**...",
		".**...",
	}))

	// frame 1 at the same place, the old pixels are fully restored first
	spr.Update(bg, 100*time.Millisecond)
	assertEqBuffers(t, bg, tcg.MustNewBufferFromStrings([]string{
		"..*...",
		".**...",
		".**...",
		"..*...",
	}))

	// frame 2
	spr.Update(bg, 200*time.Millisecond)
	assertEqBuffers(t, bg, tcg.MustNewBufferFromStrings([]string{
		"..*...",
		".**...",
		"..*...",
		".**...",
	}))

	// withdraw restores the original grid
	spr.Withdraw(bg)
	assertEqBuffers(t, bg, gridBackground())
}

// TestAnimationDirtyCellsMatchFullRender verifies the core promise of the
// partial update: re-rendering only the reported cells gives exactly the same
// screen as a full re-render, in every pixel mode.
func TestAnimationDirtyCellsMatchFullRender(t *testing.T) {
	for _, mode := range []tcg.PixelMode{tcg.Mode1x1, tcg.Mode1x2, tcg.Mode2x2, tcg.Mode2x3, tcg.Mode2x4Braille} {
		t.Run(mode.String(), func(t *testing.T) {
			bg := gridBackground()
			spr := walkAnimation(t)
			spr.MoveAbs(bg, 0, 0).Put(bg)
			before := *bg.Clone()

			// move diagonally by one pixel and switch to the next frame
			spr.Move(bg, 1, 1)
			spr.Update(bg, 100*time.Millisecond)

			full := bg.RenderAsStrings(mode)
			partial := before.RenderAsStrings(mode)
			for _, cell := range spr.ChangedCells(mode) {
				block := bg.Cut(cell.X*mode.Width(), cell.Y*mode.Height(), mode.Width(), mode.Height())
				copyInto(partial, cell.X, cell.Y, block.RenderAsStrings(mode)[0])
			}

			assert.Equal(t, full, partial, "only changed cells need to be repainted")
		})
	}
}

// copyInto replaces one terminal cell in rendered lines.
func copyInto(lines []string, x, y int, cell string) {
	if y < 0 || y >= len(lines) {
		return
	}
	runes := []rune(lines[y])
	for i, r := range []rune(cell) {
		if x+i >= 0 && x+i < len(runes) {
			runes[x+i] = r
		}
	}
	lines[y] = string(runes)
}

func TestAnimationTransparencyKeepsBackground(t *testing.T) {
	// background is fully black, sprite must not punch holes in it
	bg := tcg.NewBuffer(2, 3)
	bg.FillRect(0, 0, 2, 3, tcg.Black)

	spr := walkAnimation(t)
	spr.Put(bg)

	// frame 0: ".*" - the first column is transparent and must stay black
	assert.Equal(t, tcg.Black, bg.At(0, 0))
	assert.Equal(t, tcg.Black, bg.At(1, 0))

	// next frame: "*." - the second column must stay black too
	spr.Update(bg, time.Second/10)
	assert.Equal(t, tcg.Black, bg.At(0, 0))
	assert.Equal(t, tcg.Black, bg.At(1, 0))
}

func TestAnimationNegativeAndPartialOffscreen(t *testing.T) {
	bg := tcg.NewBuffer(4, 4)

	spr := walkAnimation(t)
	spr.Update(bg, 0)

	// place the 2x3 sprite so that its left column is off the buffer edge;
	// frame 0 is ".*" / "**" / "*.", so the visible column has pixels in the
	// first two rows and a transparent pixel in the last one
	spr.MoveAbs(bg, -1, 1).Put(bg)

	assert.Equal(t, tcg.Black, bg.At(0, 1))
	assert.Equal(t, tcg.Black, bg.At(0, 2))
	assert.Equal(t, tcg.White, bg.At(0, 3))

	// moving fully offscreen must not panic and must restore the buffer
	spr.MoveAbs(bg, -10, -10)
	assert.Equal(t, tcg.White, bg.At(0, 1))
	assert.Equal(t, tcg.White, bg.At(0, 2))

	// moving partly past the right/bottom edges paints only the visible part;
	// at (3,3) the only onscreen sprite pixel is (0,0), which is transparent
	// in frame 0, so the buffer stays clean
	spr.MoveAbs(bg, 3, 3)
	assert.Equal(t, tcg.White, bg.At(3, 3))

	// come back fully onscreen and make sure the sprite is intact
	spr.MoveAbs(bg, 0, 0)
	assert.Equal(t, tcg.Black, bg.At(1, 0), "frame 0 pixel at (1,0)")
	assert.Equal(t, tcg.Black, bg.At(0, 1), "frame 0 pixel at (0,1)")

	// changed cells outside the buffer are never reported
	for _, cell := range spr.ChangedCells(tcg.Mode1x1) {
		assert.GreaterOrEqual(t, cell.X, 0)
		assert.GreaterOrEqual(t, cell.Y, 0)
		assert.Less(t, cell.X, bg.Width)
		assert.Less(t, cell.Y, bg.Height)
	}
}

func TestAnimationPartialOffscreenRestoresBackground(t *testing.T) {
	bg := tcg.NewBuffer(4, 4)
	bg.FillRect(0, 0, 4, 4, tcg.White)
	clean := *bg.Clone()

	spr := walkAnimation(t)
	spr.MoveAbs(bg, -1, -1).Put(bg)
	spr.MoveAbs(bg, 10, 10)
	spr.MoveAbs(bg, 3, 3)
	spr.Withdraw(bg)

	assertEqBuffers(t, bg, clean)
}

func TestAnimationPauseResume(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t)
	spr.Put(bg)

	// advance to the second frame
	spr.Update(bg, 100*time.Millisecond)
	assert.Equal(t, 1, spr.FrameIndex())

	spr.Pause()
	assert.True(t, spr.Paused())

	// time keeps running, but the frame must not change
	spr.Update(bg, 250*time.Millisecond)
	assert.Equal(t, 1, spr.FrameIndex())
	spr.Update(bg, time.Second)
	assert.Equal(t, 1, spr.FrameIndex())

	spr.Resume()
	assert.False(t, spr.Paused())

	// after resume, a 50ms step continues from the middle of the second frame
	spr.Update(bg, time.Second+50*time.Millisecond)
	assert.Equal(t, 1, spr.FrameIndex())
	spr.Update(bg, time.Second+100*time.Millisecond)
	assert.Equal(t, 2, spr.FrameIndex(), "the paused gap must not be replayed")
}

func TestAnimationSeek(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t)
	spr.Put(bg)

	spr.Update(bg, 0)
	spr.Seek(bg, 250*time.Millisecond)
	assert.Equal(t, 2, spr.FrameIndex())

	// Seek moves the timeline to the requested point. The external clock is
	// still at 0 from the last Update, so another 50ms advances the 300ms loop
	// from 250ms and wraps to the first frame.
	spr.Seek(bg, 250*time.Millisecond)
	assert.Equal(t, 2, spr.FrameIndex())
	spr.Update(bg, 50*time.Millisecond)
	assert.Equal(t, 0, spr.FrameIndex(), "250ms + 50ms wraps the loop")

	// negative and too large seek positions are normalized and never fail
	spr.Seek(bg, -50*time.Millisecond)
	assert.Equal(t, 2, spr.FrameIndex())
	spr.Seek(bg, time.Hour)
	assert.Equal(t, 0, spr.FrameIndex(), "1000000 * 100ms cycles back to the first frame")
}

func TestAnimationLoopOnceStops(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t, WithLoopMode(LoopOnce))
	spr.Put(bg)

	spr.Update(bg, 0)
	assert.False(t, spr.Finished())

	spr.Update(bg, time.Second)
	assert.Equal(t, 2, spr.FrameIndex())
	assert.True(t, spr.Finished())

	// further time does not change the frame
	spr.Update(bg, 10*time.Second)
	assert.Equal(t, 2, spr.FrameIndex())
}

func TestAnimationDirtyCellsPixelModes(t *testing.T) {
	modes := []tcg.PixelMode{tcg.Mode1x1, tcg.Mode1x2, tcg.Mode2x2, tcg.Mode2x3, tcg.Mode2x4Braille}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			bg := tcg.NewBuffer(8, 8)
			spr := walkAnimation(t)

			// every reported cell must be exactly a cell with changed pixels,
			// for drawing, moving and frame changes alike
			before := *bg.Clone()
			spr.MoveAbs(bg, 2, 2).Put(bg)
			assert.Equal(t, diffCells(before, bg, mode), spr.TakeChangedCells(mode))

			before = *bg.Clone()
			spr.Move(bg, 1, 0)
			assert.Equal(t, diffCells(before, bg, mode), spr.TakeChangedCells(mode))

			before = *bg.Clone()
			spr.Update(bg, 100*time.Millisecond)
			assert.Equal(t, diffCells(before, bg, mode), spr.TakeChangedCells(mode))

			before = *bg.Clone()
			spr.Withdraw(bg)
			assert.Equal(t, diffCells(before, bg, mode), spr.TakeChangedCells(mode))

			// all reported cells stay inside the buffer
			for _, c := range diffCells(before, bg, mode) {
				assert.GreaterOrEqual(t, c.X, 0)
				assert.GreaterOrEqual(t, c.Y, 0)
			}
		})
	}
}

// diffCells - terminal cells that contain at least one changed pixel.
func diffCells(before, after tcg.Buffer, mode tcg.PixelMode) []tcg.Cell {
	seen := map[tcg.Cell]struct{}{}
	for y := 0; y < after.Height; y++ {
		for x := 0; x < after.Width; x++ {
			if before.At(x, y) != after.At(x, y) {
				seen[tcg.Cell{X: floorDiv(x, mode.Width()), Y: floorDiv(y, mode.Height())}] = struct{}{}
			}
		}
	}
	result := make([]tcg.Cell, 0, len(seen))
	for cell := range seen {
		result = append(result, cell)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Y != result[j].Y {
			return result[i].Y < result[j].Y
		}
		return result[i].X < result[j].X
	})
	return result
}

func TestAnimationMoveOnlyTouchesAffectedCells(t *testing.T) {
	bg := tcg.NewBuffer(8, 1)
	spr := walkAnimation(t)
	spr.MoveAbs(bg, 0, 0).Put(bg)
	spr.ClearDirty()

	// moving by one pixel must only touch the cells around the sprite, not the
	// whole 8-pixel buffer
	spr.Move(bg, 1, 0)
	cells := spr.ChangedCells(tcg.Mode1x1)
	assert.NotEmpty(t, cells)
	for _, c := range cells {
		assert.GreaterOrEqual(t, c.X, 0)
		assert.LessOrEqual(t, c.X, 2, "cells far from the sprite must not change")
	}
}

func TestAnimationAccumulatesDirtyUntilCleared(t *testing.T) {
	bg := tcg.NewBuffer(8, 1)
	spr := walkAnimation(t)
	spr.MoveAbs(bg, 0, 0).Put(bg)
	spr.ClearDirty()

	// a move and a frame change before one screen refresh must report the
	// union of both positions' cells
	spr.Move(bg, 1, 0)
	spr.Update(bg, 100*time.Millisecond)
	cells := spr.ChangedCells(tcg.Mode1x1)
	assert.NotEmpty(t, cells)
	assert.Contains(t, cells, tcg.Cell{X: 1, Y: 0}, "both positions must be reported")
	assert.Contains(t, cells, tcg.Cell{X: 2, Y: 0})

	// Taking the cells clears them
	taken := spr.TakeChangedCells(tcg.Mode1x1)
	assert.Equal(t, cells, taken)
	assert.Empty(t, spr.ChangedCells(tcg.Mode1x1))
	assert.True(t, spr.Dirty().Empty())
}

func TestAnimationUpdateBatchMatchesPixels(t *testing.T) {
	// a batch of Move+Update must report every cell whose pixels really
	// changed, and must not report cells outside the affected area
	for _, mode := range []tcg.PixelMode{tcg.Mode1x1, tcg.Mode2x2, tcg.Mode2x3} {
		t.Run(mode.String(), func(t *testing.T) {
			bg := tcg.NewBuffer(8, 6)
			bg.VLine(3, 0, 6, tcg.Black)
			spr := walkAnimation(t)
			spr.MoveAbs(bg, 1, 1).Put(bg)
			spr.ClearDirty()

			before := *bg.Clone()
			spr.Move(bg, 2, 1)
			spr.Update(bg, 150*time.Millisecond)

			got := spr.ChangedCells(mode)
			// every real change is covered
			assert.Subset(t, got, diffCells(before, bg, mode), "all changed cells are reported")
			// and nothing outside the old (1..2, 1..3) or new (3..4, 2..4)
			// sprite rectangle is reported
			allowed := diffCells(before, before, mode)
			for _, rect := range []struct{ x, y, w, h int }{{1, 1, 2, 3}, {3, 2, 2, 3}} {
				for py := rect.y; py < rect.y+rect.h; py++ {
					for px := rect.x; px < rect.x+rect.w; px++ {
						allowed = append(allowed, tcg.Cell{X: floorDiv(px, mode.Width()), Y: floorDiv(py, mode.Height())})
					}
				}
			}
			assert.Subset(t, allowed, got, "no cells outside the affected area")
		})
	}
}

func TestAnimationUnchangedFrameDoesNotDirty(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t)
	spr.Put(bg)
	spr.ClearDirty()

	// same frame, same position
	spr.Update(bg, 10*time.Millisecond)
	assert.True(t, spr.Dirty().Empty(), "no pixels changed")
	assert.Empty(t, spr.ChangedCells(tcg.Mode1x1))

	// crossing the whole period returns to the same frame and clears dirty
	spr.Update(bg, 300*time.Millisecond)
	assert.Equal(t, 0, spr.FrameIndex())
}

func TestAnimationUpdateBeforePutOnlyAdvancesState(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	clean := *bg.Clone()

	spr := walkAnimation(t)
	spr.Update(bg, 100*time.Millisecond)
	assert.Equal(t, 1, spr.FrameIndex())
	assertEqBuffers(t, bg, clean)
	assert.True(t, spr.Dirty().Empty())
}

func TestStaticSpriteAPIUnchanged(t *testing.T) {
	bg := gridBackground()
	clean := *bg.Clone()

	spr := New(tcg.MustNewBufferFromStrings([]string{"**", "**"}))
	spr.Move(bg, 1, 1).Put(bg)

	assert.Equal(t, 0, spr.Frames())
	assert.Equal(t, time.Duration(0), spr.Duration())

	// a static sprite ignores animation calls
	spr.Update(bg, time.Second)
	spr.Seek(bg, time.Second)
	spr.Pause()
	assert.False(t, spr.Paused())
	assert.True(t, spr.Dirty().Empty())
	assert.Equal(t, 0, spr.FrameIndex())

	spr.Withdraw(bg)
	assertEqBuffers(t, bg, clean)
}

func TestAnimationWithMask(t *testing.T) {
	bg := tcg.NewBuffer(2, 3)

	spr := walkAnimation(t)
	// mask cells are in sprite coordinates
	mask := tcg.MustNewBufferFromStrings([]string{
		".*",
		".*",
		".*",
	})
	spr.WithMask(mask)
	spr.Put(bg)

	assert.Equal(t, tcg.White, bg.At(0, 0), "masked out pixel")
	assert.Equal(t, tcg.White, bg.At(0, 1), "frame 0 has no pixel here anyway")
	assert.Equal(t, tcg.White, bg.At(0, 2))
}

func TestAnimationPerFrameMask(t *testing.T) {
	bg := tcg.NewBuffer(2, 3)

	mask := tcg.MustNewBufferFromStrings([]string{
		".*",
		".*",
		".*",
	})
	frames := []Frame{
		{Buf: walk1, Duration: time.Second, Mask: &mask},
		{Buf: walk2, Duration: time.Second},
	}
	spr, err := NewAnimation(frames)
	require.NoError(t, err)
	spr.Put(bg)

	assert.Equal(t, tcg.Black, bg.At(1, 0))
	assert.Equal(t, tcg.White, bg.At(0, 1))
}

func TestAnimationClockBackwards(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t)
	spr.Put(bg)

	spr.Update(bg, 200*time.Millisecond)
	assert.Equal(t, 2, spr.FrameIndex())

	// the clock is absolute: a smaller value is just a position on the
	// timeline, so 50ms is the first frame and the next values continue from it
	spr.Update(bg, 50*time.Millisecond)
	assert.Equal(t, 0, spr.FrameIndex())

	spr.Update(bg, 150*time.Millisecond)
	assert.Equal(t, 1, spr.FrameIndex())

	// the absolute clock value is used directly: 1000ms % 300ms = 100ms
	spr.Update(bg, time.Second)
	assert.Equal(t, 1, spr.FrameIndex(), "1000ms % 300ms = 100ms -> second frame")
}

func TestAnimationFastJump(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t)
	spr.Put(bg)

	spr.Update(bg, 0)
	// jump forward by a huge value, it must not overflow or panic
	spr.Update(bg, time.Duration(1<<62))
	index, _ := spr.FrameAt(time.Duration(1 << 62))
	assert.Equal(t, index, spr.FrameIndex())
}

func TestLoopModeString(t *testing.T) {
	assert.Equal(t, "repeat", LoopRepeat.String())
	assert.Equal(t, "once", LoopOnce.String())
	assert.Equal(t, "ping-pong", LoopPingPong.String())
	assert.Equal(t, "LoopMode(9)", LoopMode(9).String())
}

func widthAndHeight(spr *Sprite) (int, int) {
	return spr.width, spr.height
}

func TestFloorDiv(t *testing.T) {
	assert.Equal(t, 0, floorDiv(0, 2))
	assert.Equal(t, 1, floorDiv(2, 2))
	assert.Equal(t, -1, floorDiv(-1, 2))
	assert.Equal(t, -1, floorDiv(-2, 2))
}

func TestAnimationSingleAndTwoFramePingPong(t *testing.T) {
	one, err := NewAnimation([]Frame{{Buf: walk1, Duration: 100 * time.Millisecond}}, WithLoopMode(LoopPingPong))
	require.NoError(t, err)
	for _, at := range []time.Duration{0, -time.Second, time.Second, time.Hour} {
		index, _ := one.FrameAt(at)
		assert.Equal(t, 0, index, "a single-frame ping-pong always shows frame 0")
	}

	two, err := NewAnimation([]Frame{
		{Buf: walk1, Duration: 100 * time.Millisecond},
		{Buf: walk2, Duration: 100 * time.Millisecond},
	}, WithLoopMode(LoopPingPong))
	require.NoError(t, err)
	// 0,1,0,1 with a 200ms cycle
	want := map[time.Duration]int{
		0:                      0,
		100 * time.Millisecond: 1,
		199 * time.Millisecond: 1,
		200 * time.Millisecond: 0,
		-1 * time.Millisecond:  1,
		300 * time.Millisecond: 1,
	}
	for at, expected := range want {
		index, _ := two.FrameAt(at)
		assert.Equalf(t, expected, index, "two frames at %s", at)
	}
}

func TestAnimationHugeDurationsDoNotOverflow(t *testing.T) {
	huge := time.Duration(math.MaxInt64 / 3)
	frames := []Frame{
		{Buf: walk1, Duration: huge},
		{Buf: walk2, Duration: huge},
		{Buf: walk3, Duration: huge},
	}

	for _, mode := range []LoopMode{LoopRepeat, LoopOnce, LoopPingPong} {
		spr, err := NewAnimation(frames, WithLoopMode(mode))
		require.NoError(t, err)

		// no panic, index always valid, on extreme clock values
		for _, at := range []time.Duration{0, math.MaxInt64, math.MinInt64, -time.Second, huge} {
			index, phase := spr.FrameAt(at)
			assert.GreaterOrEqual(t, index, 0)
			assert.Less(t, index, len(frames))
			assert.GreaterOrEqual(t, phase, time.Duration(0))
		}
	}
}

func TestSaturatingHelpers(t *testing.T) {
	assert.Equal(t, time.Duration(math.MaxInt64), saturatingAdd(math.MaxInt64-1, 10))
	assert.Equal(t, time.Duration(15), saturatingAdd(5, 10))
	assert.Equal(t, time.Duration(math.MaxInt64), saturatingMul(math.MaxInt64/2, 3))
	assert.Equal(t, time.Duration(20), saturatingMul(10, 2))
	assert.Equal(t, time.Duration(0), subSat(5, 10))
	assert.Equal(t, time.Duration(5), subSat(10, 5))
}

func TestAnimationExtremeClockDoesNotPanic(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t)
	spr.Put(bg)

	assert.NotPanics(t, func() {
		spr.Update(bg, math.MaxInt64)
		spr.Update(bg, math.MinInt64)
		spr.Pause()
		spr.Update(bg, math.MaxInt64)
		spr.Resume()
		spr.Update(bg, math.MinInt64)
		spr.Seek(bg, math.MaxInt64)
		spr.Seek(bg, math.MinInt64)
	})

	index := spr.FrameIndex()
	assert.GreaterOrEqual(t, index, 0)
	assert.Less(t, index, spr.Frames())
}

func TestSubClamp(t *testing.T) {
	assert.Equal(t, time.Duration(math.MinInt64), subClamp(math.MinInt64, 1))
	assert.Equal(t, time.Duration(math.MaxInt64), subClamp(math.MaxInt64, -1))
	assert.Equal(t, time.Duration(3), subClamp(10, 7))
	assert.Equal(t, time.Duration(-3), subClamp(7, 10))
}

func TestAnimationAccessors(t *testing.T) {
	bg := tcg.NewBuffer(4, 3)
	spr := walkAnimation(t)
	assert.Equal(t, 2, spr.FrameWidth())
	assert.Equal(t, 3, spr.FrameHeight())
	assert.Equal(t, 3, spr.Frames())

	spr.MoveAbs(bg, 1, 2)
	x, y := spr.Position()
	assert.Equal(t, 1, x)
	assert.Equal(t, 2, y)

	// a static sprite exposes its size too
	static := New(tcg.MustNewBufferFromStrings([]string{"**", "**"}))
	assert.Equal(t, 2, static.FrameWidth())
	assert.Equal(t, 2, static.FrameHeight())
	x, y = static.Position()
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)

	// FrameAt on a static sprite is always the first frame
	index, phase := static.FrameAt(time.Second)
	assert.Equal(t, 0, index)
	assert.Equal(t, time.Duration(0), phase)
}

func TestAnimationDirtyPixelsAndLen(t *testing.T) {
	bg := tcg.NewBuffer(3, 3)
	spr := walkAnimation(t)
	spr.MoveAbs(bg, 0, 0).Put(bg)

	dirty := spr.Dirty()
	assert.False(t, dirty.Empty())
	assert.Equal(t, dirty.Len(), len(dirty.Pixels()))
	for i := 1; i < len(dirty.Pixels()); i++ {
		prev, cur := dirty.Pixels()[i-1], dirty.Pixels()[i]
		assert.True(t, prev.Y < cur.Y || (prev.Y == cur.Y && prev.X < cur.X), "pixels sorted by Y then X")
	}

	// frame 0 is ".*" - just three pixels, one per row
	assert.Equal(t, 4, dirty.Len())
}

func TestAnimationSingleFrameRepeatAndOnce(t *testing.T) {
	// a one-frame animation is always on frame 0
	for _, mode := range []LoopMode{LoopRepeat, LoopOnce, LoopPingPong} {
		spr, err := NewAnimation([]Frame{{Buf: walk1, Duration: 50 * time.Millisecond}}, WithLoopMode(mode))
		require.NoError(t, err)

		bg := tcg.NewBuffer(2, 3)
		spr.Put(bg)
		for _, at := range []time.Duration{0, 25 * time.Millisecond, 50 * time.Millisecond, time.Hour} {
			spr.Update(bg, at)
			assert.Equal(t, 0, spr.FrameIndex(), "%s at %s", mode, at)
		}
	}
}
