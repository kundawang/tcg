/*
Package sprite - sprite object for drawing/moving on tcg.Buffer

A sprite can be static (created with New) or animated (created with
NewAnimation). Both kinds share the same movement API: Put, Withdraw,
Move and MoveAbs, and the static API behaves exactly as before.

An animated sprite owns frames with individual durations and a loop mode.
Time is supplied by the caller, so animations are fully deterministic and
can be tested with a fixed clock:

	frames := []sprite.Frame{
		{Buf: step1, Duration: 100 * time.Millisecond},
		{Buf: step2, Duration: 100 * time.Millisecond},
		{Buf: step3, Duration: 100 * time.Millisecond},
	}
	spr, err := sprite.NewAnimation(frames, sprite.WithLoopMode(sprite.LoopRepeat))
	if err != nil {
		// handle error
	}

	// tick is an absolute point on the caller's clock (time since start), not a delta
	spr.Update(bg, tick)
	tg.UpdateCells(spr.ChangedCells(tg.Mode())...) // repaint only changed cells

Drawing is transparent: White pixels of a frame keep the background, so each
frame can have its own silhouette. Only the pixels touched by the sprite are
reported by Dirty/ChangedCells, which keeps a moving or changing sprite cheap
even for large buffers.
*/
package sprite

import (
	"time"

	"github.com/msoap/tcg"
)

// Sprite - sprite object
type Sprite struct {
	Buf     tcg.Buffer // sprite image, for an animated sprite it is the current frame
	x, y    int        // position on buffer, can be negative
	width   int
	height  int
	mask    *tcg.Buffer // mask for sprite
	bg      tcg.Buffer  // saved background under the drawn image
	isDrawn bool        // is sprite drawn on buffer

	// animation state, frames is empty for a static sprite
	frames       []Frame
	loop         LoopMode
	frameIndex   int
	animTime     time.Duration // position on the animation timeline
	clockBase    time.Duration // external clock value that maps to timeline zero
	lastRaw      time.Duration // last external clock value seen by Update
	clockStarted bool
	paused       bool
	pausedAt     time.Duration // external clock value at Pause

	// position and frame of the last painted image, used to restore exactly
	// what was drawn and to skip a repaint when nothing changed
	drawnX     int
	drawnY     int
	drawnFrame int

	// pixels changed since the last ClearDirty call
	lastDirty Dirty
}

// New - get new sprite object from tcg.Buffer
func New(buf tcg.Buffer) *Sprite {
	return &Sprite{
		width:  buf.Width,
		height: buf.Height,
		Buf:    buf,
		bg:     tcg.NewBuffer(buf.Width, buf.Height),
	}
}

// WithMask - add mask to sprite. For an animated sprite the mask is shared by
// all frames, but a frame can override it with Frame.Mask.
func (s *Sprite) WithMask(mask tcg.Buffer) *Sprite {
	s.mask = &mask
	return s
}

// Put - put sprite on buffer, save background under sprite, change state of sprite to drawn.
// For an animated sprite the current frame is drawn and the animation keeps
// its timeline; use Update to advance it afterwards.
func (s *Sprite) Put(buf tcg.Buffer) *Sprite {
	if s.isAnimated() {
		// force the repaint: the background under the sprite may have changed
		// since the last Put, so the frame is redrawn over fresh pixels
		s.render(buf, s.frameIndex, true)
		return s
	}

	s.draw(buf)
	s.isDrawn = true

	return s
}

// draw - draw sprite on buffer, save background under sprite
func (s *Sprite) draw(buf tcg.Buffer) {
	// copy background
	s.bg.BitBlt(0, 0, s.width, s.height, buf, s.x, s.y)

	// draw sprite
	var opts []tcg.BitBltOpt
	if s.mask != nil {
		opts = append(opts, tcg.BBMask(s.mask))
	}
	buf.BitBlt(s.x, s.y, s.width, s.height, s.Buf, 0, 0, opts...)
}

// Withdraw - withdraw sprite from buffer
func (s *Sprite) Withdraw(buf tcg.Buffer) *Sprite {
	if !s.isDrawn {
		return s
	}

	if s.isAnimated() {
		s.withdraw(buf)
		return s
	}

	s.clear(buf)
	s.isDrawn = false

	return s
}

// clear sprite on buffer
func (s *Sprite) clear(buf tcg.Buffer) {
	buf.BitBlt(s.x, s.y, s.width, s.height, s.bg, 0, 0)
}

// MoveAbs - move sprite on buffer to absolute position, coordinates can be negative
func (s *Sprite) MoveAbs(buf tcg.Buffer, x, y int) *Sprite {
	if s.isAnimated() {
		s.x, s.y = x, y
		if s.isDrawn {
			s.render(buf, s.frameIndex, false)
		}
		return s
	}

	if s.isDrawn {
		s.clear(buf)
	}

	s.x = x
	s.y = y

	if s.isDrawn {
		s.draw(buf)
	}

	return s
}

// Move - move sprite on buffer to relative position
func (s *Sprite) Move(buf tcg.Buffer, x, y int) *Sprite {
	return s.MoveAbs(buf, s.x+x, s.y+y)
}
